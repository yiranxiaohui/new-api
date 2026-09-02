export const meta = {
  apiVersion: 1,
  key: "newapi-video",
  name: "New API Video",
  icon: "OpenAI",
  description: {
    en: "Upstreams that speak the New API video protocol (POST /v1/video/generations, GET /v1/videos/{id})",
    zh: "以 New API 视频协议（POST /v1/video/generations、GET /v1/videos/{id}）暴露的中转上游",
  },
  version: "1.0.0",
  channelTypes: [62],
  author: { name: "yiranxiaohui" },
  models: ["veo-3.1-quality", "veo-3.1-fast", "veo-3.1-lite", "veo3.1", "veo3.1-fast", "veo3.1-lite"],
  fetchMode: "per_task",
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: {
        en: "Requested video duration in seconds (display only; this upstream is billed per call).",
        zh: "请求的视频时长，单位为秒（仅展示；该类上游按次固定计费）。",
      },
    },
  },
  protocols: [{ name: "openai_responses", supports: ["stream", "sync", "background"] }, "openai_video"],
};

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

function requestSeconds(req) {
  const raw = req.seconds === undefined ? req.duration : req.seconds;
  if (raw === undefined || raw === null || raw === "") return undefined;
  const seconds = Number(raw);
  if (!Number.isFinite(seconds) || seconds <= 0 || seconds > 3600) throw new Error("seconds must be between 1 and 3600");
  return seconds;
}

// Files are never inlined by this plugin: the New API video protocol only
// accepts JSON, so multipart uploads are rejected at decode time.
export function buildSubmitRequest(ctx) {
  const req = ctx.requestBody || {};
  if (!trimmed(req.prompt)) throw new Error("field prompt is required");
  if (ctx.action === "remix") throw new Error("remix is not supported by this channel type");
  const body = Object.assign({}, req, { model: ctx.upstreamModel || ctx.model });
  const seconds = requestSeconds(req);
  if (seconds !== undefined) body.seconds = String(seconds);
  delete body.duration;
  return {
    url: ctx.baseUrl + "/v1/video/generations",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", Accept: "application/json" },
    body: body,
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp.body || {};
  const taskId = trimmed(body.id || body.task_id);
  if (!taskId) throw new Error("task_id is empty");
  return { taskId: taskId, taskData: body };
}

// Billing is a fixed per-call price configured on the model; no multipliers.
export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return {};
  const seconds = requestSeconds(ctx.requestBody || {});
  return seconds === undefined ? {} : { seconds: seconds };
}

export function extractUsageOnComplete(_task, _taskResult, body) {
  const seconds = Number((body || {}).seconds || (body || {}).duration || 0);
  if (!Number.isFinite(seconds) || seconds <= 0) return {};
  return { seconds: Math.min(seconds, 3600) };
}

export function buildQueryRequest(ctx) {
  return {
    url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" },
  };
}

// New API style upstreams disagree on where the finished video lives; accept
// the common shapes (url, video_url, videos[0].url, data.url, metadata.url).
function firstVideoURL(body) {
  if (!body || typeof body !== "object") return "";
  const direct = [body.url, body.video_url];
  for (const candidate of direct) {
    if (typeof candidate === "string" && trimmed(candidate)) return trimmed(candidate);
    if (candidate && typeof candidate === "object" && typeof candidate.url === "string" && trimmed(candidate.url)) return trimmed(candidate.url);
  }
  if (Array.isArray(body.videos) && body.videos.length) {
    const first = body.videos[0];
    if (typeof first === "string" && trimmed(first)) return trimmed(first);
    if (first && typeof first === "object" && typeof first.url === "string" && trimmed(first.url)) return trimmed(first.url);
  }
  for (const key of ["data", "metadata"]) {
    const nested = body[key];
    if (nested && typeof nested === "object" && !Array.isArray(nested) && typeof nested.url === "string" && trimmed(nested.url)) {
      return trimmed(nested.url);
    }
  }
  return "";
}

export function parseTaskResult(ctx, body) {
  const statuses = {
    queued: "QUEUED",
    pending: "QUEUED",
    processing: "IN_PROGRESS",
    in_progress: "IN_PROGRESS",
    completed: "SUCCESS",
    succeeded: "SUCCESS",
    failed: "FAILURE",
    cancelled: "FAILURE",
  };
  const result = { status: statuses[body.status] || "UNKNOWN" };
  if (body.progress > 0 && body.progress < 100) result.progress = body.progress + "%";
  if (result.status === "SUCCESS") {
    const url = firstVideoURL(body);
    if (url) result.url = url;
  }
  if (result.status === "FAILURE") result.reason = body.error && body.error.message ? body.error.message : "task failed";
  return result;
}

function artifactVideoURL(task) {
  return firstVideoURL(task && task.data);
}

export function listArtifacts(task) {
  return task.status === "SUCCESS" ? [{ key: "video", type: "video" }] : [];
}

// Prefer the stored absolute URL from the query response. Upstreams that
// return their own /v1/videos/{id}/content endpoint (or none at all) are
// proxied through the channel base URL with the channel credential, so a
// loopback or private address embedded in the upstream response is never
// trusted as-is.
export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const stored = artifactVideoURL(ctx);
  const upstreamTaskId = trimmed(ctx.upstreamTaskId);
  const contentSuffix = "/v1/videos/" + encodeURIComponent(upstreamTaskId) + "/content";
  const isContentEndpoint = stored && upstreamTaskId && stored.replace(/\/+$/, "").endsWith(contentSuffix);
  if (stored && !isContentEndpoint) {
    return { url: stored, method: ctx.clientRequest.method, credentialless: true };
  }
  if (!upstreamTaskId) throw new Error("artifact_not_found");
  return {
    url: ctx.baseUrl + contentSuffix,
    method: ctx.clientRequest.method,
    headers: { Authorization: "Bearer " + ctx.apiKey },
  };
}

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
      if (req.metadata !== undefined && (!req.metadata || typeof req.metadata !== "object" || Array.isArray(req.metadata)))
        throw new Error("metadata must be an object");
      const input = responsesInput(req);
      const prompt = input.prompt || trimmed(req.prompt);
      if (!prompt) throw new Error("input is required");
      const images = [];
      for (const image of [req.image, req.input_reference].concat(req.images || [], input.images)) {
        if (trimmed(image) && !images.includes(trimmed(image))) images.push(trimmed(image));
      }
      const requestBody = { model: model, prompt: prompt };
      if (images.length) requestBody.input_reference = images[0];
      if (Object.prototype.hasOwnProperty.call(req, "seconds")) requestBody.seconds = req.seconds;
      else if (Object.prototype.hasOwnProperty.call(req, "duration")) requestBody.seconds = req.duration;
      if (Object.prototype.hasOwnProperty.call(req, "size")) requestBody.size = req.size;
      if (Object.prototype.hasOwnProperty.call(req, "metadata")) requestBody.metadata = req.metadata;
      requestSeconds(requestBody);
      return { kind: "submit", model: model, action: images.length ? "image_to_video" : "text_to_video", requestBody: requestBody };
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
        metadata: { vendor: "newapi-video" },
      };
    },
  },
  openai_video: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("this channel type only accepts JSON bodies");
      if (!ctx.body.value || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
      const req = ctx.body.value;
      const requestBody = Object.assign({}, req, { model: ctx.model });
      requestSeconds(requestBody);
      return {
        kind: "submit",
        model: ctx.model,
        action: req.input_reference || req.image ? "image_to_video" : "text_to_video",
        requestBody: requestBody,
      };
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
