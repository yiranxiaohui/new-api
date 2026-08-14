package xai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

const (
	requestContextKey      = "xai_video_request"
	defaultDurationSeconds = 8
)

var modelList = []string{
	"grok-imagine-video",
	"grok-imagine-video-1.5",
}

type mediaInput struct {
	URL    *string `json:"url,omitempty"`
	FileID *string `json:"file_id,omitempty"`
}

type generationRequest struct {
	Model           string        `json:"model"`
	Prompt          *string       `json:"prompt,omitempty"`
	Duration        *dto.IntValue `json:"duration,omitempty"`
	Seconds         *dto.IntValue `json:"seconds,omitempty"`
	AspectRatio     *string       `json:"aspect_ratio,omitempty"`
	Resolution      *string       `json:"resolution,omitempty"`
	Image           *mediaInput   `json:"image,omitempty"`
	ReferenceImages []mediaInput  `json:"reference_images,omitempty"`
}

type generationResponse struct {
	RequestID string `json:"request_id"`
}

type resultResponse struct {
	Status   string          `json:"status"`
	Model    string          `json:"model,omitempty"`
	Video    *videoResult    `json:"video,omitempty"`
	Progress *float64        `json:"progress,omitempty"`
	Error    json.RawMessage `json:"error,omitempty"`
}

type videoResult struct {
	URL string `json:"url"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		return service.TaskErrorWrapperLocal(errors.New("xAI video generation requires application/json"), "invalid_request", http.StatusBadRequest)
	}

	var req generationRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(errors.New("model field is required"), "missing_model", http.StatusBadRequest)
	}

	for _, duration := range []*dto.IntValue{req.Duration, req.Seconds} {
		if duration == nil {
			continue
		}
		seconds := int(*duration)
		if seconds < 1 || seconds > relaycommon.MaxTaskDurationSeconds {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("duration must be between 1 and %d", relaycommon.MaxTaskDurationSeconds),
				"invalid_duration",
				http.StatusBadRequest,
			)
		}
	}

	prompt := ""
	if req.Prompt != nil {
		prompt = strings.TrimSpace(*req.Prompt)
	}
	if req.Image != nil && len(req.ReferenceImages) > 0 {
		return service.TaskErrorWrapperLocal(errors.New("image and reference_images cannot be used together"), "invalid_request", http.StatusBadRequest)
	}
	if req.Image == nil && prompt == "" {
		return service.TaskErrorWrapperLocal(errors.New("prompt is required without an image"), "invalid_request", http.StatusBadRequest)
	}
	if len(req.ReferenceImages) > 0 && prompt == "" {
		return service.TaskErrorWrapperLocal(errors.New("prompt is required with reference_images"), "invalid_request", http.StatusBadRequest)
	}

	info.Action = constant.TaskActionTextGenerate
	if req.Image != nil {
		info.Action = constant.TaskActionGenerate
	} else if len(req.ReferenceImages) > 0 {
		info.Action = constant.TaskActionReferenceGenerate
	}
	c.Set(requestContextKey, req)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	value, exists := c.Get(requestContextKey)
	if !exists {
		return nil
	}
	req, ok := value.(generationRequest)
	if !ok {
		return nil
	}

	duration := defaultDurationSeconds
	if req.Duration != nil {
		duration = int(*req.Duration)
	} else if req.Seconds != nil {
		duration = int(*req.Seconds)
	}

	resolution := "480p"
	if req.Resolution != nil && strings.TrimSpace(*req.Resolution) != "" {
		resolution = strings.ToLower(strings.TrimSpace(*req.Resolution))
	}
	modelName := info.UpstreamModelName
	if modelName == "" {
		modelName = req.Model
	}

	return map[string]float64{
		"seconds":    float64(duration),
		"resolution": resolutionRatio(modelName, resolution),
	}
}

func resolutionRatio(modelName, resolution string) float64 {
	if strings.HasPrefix(modelName, "grok-imagine-video-1.5") {
		switch resolution {
		case "720p":
			return 1.75
		case "1080p":
			return 3.125
		}
		return 1
	}
	if resolution == "720p" {
		return 1.4
	}
	return 1
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/videos/generations", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get request body failed")
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read request body failed")
	}
	body, err = sjson.SetBytes(body, "model", info.UpstreamModelName)
	if err != nil {
		return nil, errors.Wrap(err, "set upstream model failed")
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var upstream generationResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(upstream.RequestID) == "" {
		taskErr = service.TaskErrorWrapper(errors.New("request_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, generationResponse{RequestID: info.PublicTaskID})
	return upstream.RequestID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, errors.New("invalid task_id")
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/videos/%s", baseURL, url.PathEscape(taskID)), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, errors.Wrap(err, "create http client")
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var result resultResponse
	if err := common.Unmarshal(respBody, &result); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskInfo := &relaycommon.TaskInfo{}
	switch result.Status {
	case "pending":
		taskInfo.Status = model.TaskStatusQueued
	case "done":
		if result.Video == nil || strings.TrimSpace(result.Video.URL) == "" {
			return nil, errors.New("done response is missing video.url")
		}
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Url = result.Video.URL
	case "expired", "failed":
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Reason = resultFailureReason(result.Error, result.Status)
	}
	if result.Progress != nil && *result.Progress >= 0 && *result.Progress <= 100 {
		taskInfo.Progress = strconv.FormatFloat(*result.Progress, 'f', -1, 64) + "%"
	}
	return taskInfo, nil
}

func resultFailureReason(raw json.RawMessage, status string) string {
	if len(raw) > 0 && string(raw) != "null" {
		var message string
		if err := common.Unmarshal(raw, &message); err == nil && strings.TrimSpace(message) != "" {
			return message
		}
		var detail struct {
			Message string `json:"message"`
		}
		if err := common.Unmarshal(raw, &detail); err == nil && strings.TrimSpace(detail.Message) != "" {
			return detail.Message
		}
	}
	if status == "expired" {
		return "video generation expired"
	}
	return "video generation failed"
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	payload := make(map[string]any)
	if len(task.Data) > 0 {
		_ = common.Unmarshal(task.Data, &payload)
	}
	delete(payload, "request_id")

	switch task.Status {
	case model.TaskStatusSuccess:
		payload["status"] = "done"
		video, _ := payload["video"].(map[string]any)
		if video == nil {
			video = make(map[string]any)
			payload["video"] = video
		}
		if _, ok := video["url"]; !ok && task.GetResultURL() != "" {
			video["url"] = task.GetResultURL()
		}
	case model.TaskStatusFailure:
		if payload["status"] != "expired" {
			payload["status"] = "failed"
		}
		if _, ok := payload["error"]; !ok && task.FailReason != "" {
			payload["error"] = map[string]any{"message": task.FailReason}
		}
	default:
		payload["status"] = "pending"
	}
	if _, ok := payload["model"]; !ok {
		modelName := task.Properties.OriginModelName
		if modelName == "" {
			modelName = task.Properties.UpstreamModelName
		}
		if modelName != "" {
			payload["model"] = modelName
		}
	}

	return common.Marshal(payload)
}

func (a *TaskAdaptor) GetModelList() []string {
	return modelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return "xai"
}
