export const meta = {
  apiVersion: 1,
  key: "xai",
  name: "xAI Grok Imagine Video",
  icon: "XAI",
  description: {
    en: "xAI Grok Imagine video generation (text-to-video, image-to-video, reference images)",
    zh: "xAI Grok Imagine 视频生成（文生视频、图生视频、参考图）",
  },
  version: "1.0.0",
  channelTypes: [48],
  author: { name: "yiranxiaohui" },
  models: ["grok-imagine-video", "grok-imagine-video-1.5"],
  fetchMode: "per_task",
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Requested video duration in seconds.", zh: "请求的视频时长，单位为秒。" },
    },
    resolution: {
      enum: ["480p", "720p", "1080p"],
      description: { en: "Requested output video resolution.", zh: "请求的输出视频分辨率。" },
    },
  },
  // The xAI-native POST /v1/videos/generations entry is host-owned (see
  // router/video-router.go): it shares its path shape with the OpenAI video
  // retrieve route, which the plugin router refuses to register. The native
  // decode/render members below are what that host route calls.
  protocols: [{ name: "openai_responses", supports: ["stream", "sync", "background"] }, "openai_video"],
};

const DEFAULT_DURATION_SECONDS = 8;

function trimmed(value) {
  return String(value || "").trim();
}

function responsesInput(req) {
  const texts = [],
    images = [];
  const input = req.input;
  if (typeof input === "string") texts.push(input);
  else if (Array.isArray(input)) {
    for (const item of input) {
      if (typeof item === "string") {
        texts.push(item);
        continue;
      }
      if (!item || typeof item !== "object" || Array.isArray(item)) continue;
      const content = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
      for (const part of content) {
        if (typeof part === "string") {
          texts.push(part);
          continue;
        }
        if (!part || typeof part !== "object" || Array.isArray(part)) continue;
        if (["input_text", "text"].includes(part.type) && typeof part.text === "string") texts.push(part.text);
        if (["input_image", "image_url"].includes(part.type)) {
          let image = part.image_url;
          if (image && typeof image === "object") image = image.url;
          if (trimmed(image)) images.push(trimmed(image));
        }
      }
    }
  }
  return {
    prompt: texts
      .filter(function (text) {
        return trimmed(text);
      })
      .join("\n"),
    images: images,
  };
}

function responsesVideoText(ctx) {
  const artifact = ctx && ctx.artifacts && ctx.artifacts.video;
  const url = trimmed(artifact && artifact.url);
  if (!url) throw new Error("video artifact is unavailable");
  const escaped = url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + escaped + '"></video>';
}

function durationOf(value) {
  if (value === undefined || value === null || value === "") return undefined;
  const seconds = Number(value);
  if (!Number.isFinite(seconds) || seconds !== Math.trunc(seconds) || seconds < 1 || seconds > 3600) throw new Error("duration must be between 1 and 3600");
  return seconds;
}

function mediaInput(image) {
  const value = trimmed(image);
  if (!value) throw new Error("input_reference is empty");
  return value.startsWith("file_") ? { file_id: value } : { url: value };
}

// OpenAI-style "size" is translated to xAI aspect_ratio + resolution.
const SIZE_MAP = {
  "854x480": ["16:9", "480p"],
  "864x480": ["16:9", "480p"],
  "480x854": ["9:16", "480p"],
  "480x864": ["9:16", "480p"],
  "1280x720": ["16:9", "720p"],
  "720x1280": ["9:16", "720p"],
  "1792x1024": ["16:9", "1080p"],
  "1920x1080": ["16:9", "1080p"],
  "1024x1792": ["9:16", "1080p"],
  "1080x1920": ["9:16", "1080p"],
};

// Canonical request shape shared by every entry point: the xAI native
// generation body (prompt, duration, aspect_ratio, resolution, image,
// reference_images) plus the client model name.
function validateGeneration(req) {
  const body = {};
  const model = trimmed(req.model);
  if (!model) throw new Error("model field is required");
  body.model = model;
  const prompt = trimmed(req.prompt);
  if (prompt) body.prompt = prompt;
  const duration = durationOf(req.duration === undefined ? req.seconds : req.duration);
  if (duration !== undefined) body.duration = duration;
  if (req.aspect_ratio !== undefined) body.aspect_ratio = trimmed(req.aspect_ratio);
  if (req.resolution !== undefined) body.resolution = trimmed(req.resolution).toLowerCase();
  if (req.image !== undefined && req.image !== null) {
    if (!req.image || typeof req.image !== "object" || Array.isArray(req.image)) throw new Error("image must be an object");
    body.image = req.image;
  }
  if (req.reference_images !== undefined && req.reference_images !== null) {
    if (!Array.isArray(req.reference_images)) throw new Error("reference_images must be an array");
    body.reference_images = req.reference_images;
  }
  if (body.image && body.reference_images && body.reference_images.length) throw new Error("image and reference_images cannot be used together");
  if (!body.image && !prompt) throw new Error("prompt is required without an image");
  if (body.reference_images && body.reference_images.length && !prompt) throw new Error("prompt is required with reference_images");
  return body;
}

function actionOf(body) {
  if (body.image) return "image_to_video";
  if (body.reference_images && body.reference_images.length) return "reference_to_video";
  return "text_to_video";
}

function fromOpenAIVideo(req, model) {
  const converted = { model: model, prompt: trimmed(req.prompt) };
  const duration = req.duration === undefined ? req.seconds : req.duration;
  if (duration !== undefined && duration !== null && duration !== "") converted.duration = duration;
  const size = trimmed(req.size).toLowerCase();
  if (size) {
    const mapped = SIZE_MAP[size];
    if (!mapped) throw new Error("unsupported OpenAI video size for xAI: " + req.size);
    converted.aspect_ratio = mapped[0];
    converted.resolution = mapped[1];
  }
  const images = [];
  for (const image of [req.input_reference, req.image].concat(Array.isArray(req.images) ? req.images : [])) {
    if (trimmed(image) && !images.includes(trimmed(image))) images.push(trimmed(image));
  }
  if (images.length > 1) throw new Error("xAI image-to-video accepts one input_reference");
  if (images.length === 1) converted.image = mediaInput(images[0]);
  return converted;
}

function resolutionRatio(model, resolution) {
  if (String(model).startsWith("grok-imagine-video-1.5")) {
    if (resolution === "720p") return 1.75;
    if (resolution === "1080p") return 3.125;
    return 1;
  }
  return resolution === "720p" ? 1.4 : 1;
}

function usageOf(ctx) {
  const req = ctx.requestBody || {};
  const duration = req.duration === undefined ? DEFAULT_DURATION_SECONDS : Number(req.duration);
  let resolution = trimmed(req.resolution).toLowerCase() || "480p";
  if (!["480p", "720p", "1080p"].includes(resolution)) resolution = "480p";
  return { seconds: Math.min(duration, 3600), resolution: resolution };
}

export function buildSubmitRequest(ctx) {
  const req = ctx.requestBody || {};
  const body = Object.assign({}, req, { model: ctx.upstreamModel || ctx.model });
  return {
    url: ctx.baseUrl + "/v1/videos/generations",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", Accept: "application/json" },
    body: body,
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp.body || {};
  const taskId = trimmed(body.request_id);
  if (!taskId) throw new Error("request_id is empty");
  return { taskId: taskId, taskData: body };
}

// Billing multipliers live under a separate "resolution-<tier>" key: the
// declared "resolution" usage fact is an enum and must stay a string.
export function extractUsage(ctx) {
  const usage = usageOf(ctx);
  if (ctx.usagePurpose === "billing_ratios") {
    const ratios = { seconds: usage.seconds };
    ratios["resolution-" + usage.resolution] = resolutionRatio(ctx.upstreamModel || ctx.model, usage.resolution);
    return ratios;
  }
  return usage;
}

export function extractUsageOnComplete(_task, _taskResult, _body) {
  return {};
}

export function buildQueryRequest(ctx) {
  return {
    url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" },
  };
}

function failureReason(body) {
  const error = body.error;
  if (typeof error === "string" && trimmed(error)) return trimmed(error);
  if (error && typeof error === "object" && typeof error.message === "string" && trimmed(error.message)) return trimmed(error.message);
  return body.status === "expired" ? "video generation expired" : "video generation failed";
}

export function parseTaskResult(ctx, body) {
  const result = { status: "UNKNOWN" };
  switch (body.status) {
    case "pending":
      result.status = "QUEUED";
      break;
    case "done": {
      const url = trimmed(body.video && body.video.url);
      if (!url) throw new Error("done response is missing video.url");
      result.status = "SUCCESS";
      result.url = url;
      break;
    }
    case "expired":
    case "failed":
      result.status = "FAILURE";
      result.reason = failureReason(body);
      break;
    default:
      break;
  }
  const progress = Number(body.progress);
  if (Number.isFinite(progress) && progress >= 0 && progress <= 100) result.progress = progress + "%";
  return result;
}

function artifactVideoURL(task) {
  const data = task && task.data;
  return trimmed(data && data.video && data.video.url);
}

export function listArtifacts(task) {
  return task.status === "SUCCESS" && artifactVideoURL(task) ? [{ key: "video", type: "video" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const url = artifactVideoURL(ctx);
  if (!url) throw new Error("artifact_not_found");
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}

export const native = {
  decodeGeneration: function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json") throw new Error("xAI video generation requires application/json");
    const req = ctx.body.value;
    if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
    const body = validateGeneration(req);
    return { kind: "submit", model: body.model, action: actionOf(body), requestBody: body };
  },
  generationCreated: function (ctx, task) {
    return { request_id: task.task_id };
  },
  error: function (ctx, error) {
    return { code: error.code, error: error.message };
  },
};

export const protocols = {
  openai_responses: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
      const req = ctx.body.value;
      if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
      const model = trimmed(req.model);
      if (!model) throw new Error("model is required");
      if (req.input !== undefined && typeof req.input !== "string" && !Array.isArray(req.input)) throw new Error("input must be a string or array");
      if (req.images !== undefined && !Array.isArray(req.images)) throw new Error("images must be an array");
      const input = responsesInput(req);
      const prompt = input.prompt || trimmed(req.prompt);
      if (!prompt) throw new Error("input is required");
      const openai = { model: model, prompt: prompt };
      const images = [];
      for (const image of [req.image, req.input_reference].concat(req.images || [], input.images)) {
        if (trimmed(image) && !images.includes(trimmed(image))) images.push(trimmed(image));
      }
      if (images.length) openai.input_reference = images[0];
      if (Object.prototype.hasOwnProperty.call(req, "seconds")) openai.seconds = req.seconds;
      else if (Object.prototype.hasOwnProperty.call(req, "duration")) openai.duration = req.duration;
      if (Object.prototype.hasOwnProperty.call(req, "size")) openai.size = req.size;
      const body = validateGeneration(fromOpenAIVideo(openai, model));
      return { kind: "submit", model: model, action: actionOf(body), requestBody: body };
    },
    renderEvents: function (ctx, task, previousState) {
      const status = String(task.status || "UNKNOWN").toUpperCase();
      const value = Number(String(task.progress || "").replace("%", ""));
      const progress = Number.isFinite(value) && value >= 0 && value <= 100 ? value : null;
      const state = { status: status, progress: progress };
      if (status === "SUCCESS") {
        const text = responsesVideoText(ctx);
        const events = previousState && previousState.status === status ? [] : [{ type: "output", data: text }];
        return { events: events, state: state, done: true };
      }
      if (status === "FAILURE")
        return { events: [{ type: "error", code: "task_failed", message: task.fail_reason || "task failed" }], state: state, done: true };
      if (previousState && previousState.status === status && previousState.progress === progress) return { events: [], state: state, done: false };
      const event = { type: "progress", message: status.toLowerCase() };
      if (progress !== null) event.progress = progress;
      return { events: [event], state: state, done: false };
    },
    renderFinal: function (ctx, _task) {
      return {
        output: [
          {
            type: "message",
            status: "completed",
            role: "assistant",
            content: [{ type: "output_text", text: responsesVideoText(ctx), annotations: [], logprobs: [] }],
          },
        ],
        metadata: { vendor: "xai" },
      };
    },
  },
  openai_video: {
    decodeRequest: function (ctx) {
      if (!ctx.body || (ctx.body.kind !== "json" && ctx.body.kind !== "multipart")) throw new Error("JSON or multipart body required");
      let req;
      if (ctx.body.kind === "json") {
        if (!ctx.body.value || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
        req = ctx.body.value;
      } else {
        req = {};
        const fields = ctx.body.fields || {};
        for (const name of Object.keys(fields)) {
          if (fields[name].length > 1) throw new Error(name + " must be provided once");
          req[name] = fields[name][0];
        }
        for (const file of ctx.body.files || []) {
          if (file.field !== "input_reference") throw new Error("unexpected file field: " + file.field);
          if (req.input_reference !== undefined) throw new Error("xAI image-to-video accepts one input_reference");
          if (
            !String(file.mimeType || "")
              .toLowerCase()
              .startsWith("image/")
          )
            throw new Error("input_reference must be an image");
          req.input_reference = { __fileRef: file.ref, encoding: "dataUrl", mimeType: file.mimeType };
        }
      }
      const converted = fromOpenAIVideo(
        Object.assign({}, req, { input_reference: typeof req.input_reference === "string" ? req.input_reference : "" }),
        ctx.model
      );
      if (req.input_reference && typeof req.input_reference === "object") {
        if (converted.image) throw new Error("xAI image-to-video accepts one input_reference");
        converted.image = { url: req.input_reference };
      }
      const body = validateGeneration(converted);
      return { kind: "submit", model: ctx.model, action: actionOf(body), requestBody: body };
    },
    render: function (ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      const output = {
        id: task.task_id,
        object: "video",
        model: (task.properties || {}).origin_model_name || "",
        status: statuses[task.status] || "unknown",
        progress: Number(String(task.progress || "0").replace("%", "")),
        created_at: Number(task.created_at || 0),
      };
      const completedAt = Number(task.finished_at || task.updated_at || 0);
      if (completedAt > 0) output.completed_at = completedAt;
      if (task.status === "FAILURE") output.error = { code: "video_generation_failed", message: task.fail_reason || "The video generation task failed." };
      return output;
    },
  },
};
