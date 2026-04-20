#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
压测脚本：批量请求 new-api 网关，汇总状态码与错误消息。

用途：
- 验证自定义错误信息（HideUpstreamErrorMessage）是否真的生效
- 摸一下某个上游/渠道/模型的失败率
- 对比 OpenAI 协议与 Claude 协议的表现

交互式运行：
    python scripts/stress_test.py

启动后会逐项询问 base / key / protocol / model / 次数 / 并发等，
直接回车使用方括号里的默认值。

配置文件：
    脚本启动时会自动读取同目录下的 config.json（若存在），
    里面的字段会作为交互提示的默认值。把 "non_interactive": true
    并把 key 填好，即可跳过所有提示一键跑。

    config.json 字段（全部可选）：
      base, key, protocol, model, prompt, n, concurrency, timeout,
      non_interactive

    也可以用 --config <path> 指定别的配置文件位置。

只依赖 Python 3.8+ 标准库（urllib / concurrent.futures），无需 pip。
"""
from __future__ import annotations

import argparse
import collections
import concurrent.futures
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass


@dataclass
class Result:
    idx: int
    status: int           # HTTP 状态码，0 表示连接异常
    ok: bool              # 业务是否成功（2xx 且无 error 字段）
    err_type: str         # error.type
    err_msg: str          # error.message
    raw_head: str         # 响应前 200 字节，便于排查
    elapsed_ms: int


def build_openai_request(base: str, key: str, model: str, prompt: str) -> urllib.request.Request:
    url = base.rstrip("/") + "/v1/chat/completions"
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": 32,
        "stream": False,
    }).encode("utf-8")
    return urllib.request.Request(
        url,
        data=body,
        method="POST",
        headers={
            "Authorization": f"Bearer {key}",
            "Content-Type": "application/json",
        },
    )


def build_claude_request(base: str, key: str, model: str, prompt: str) -> urllib.request.Request:
    url = base.rstrip("/") + "/v1/messages"
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": 32,
        "stream": False,
    }).encode("utf-8")
    return urllib.request.Request(
        url,
        data=body,
        method="POST",
        headers={
            "Authorization": f"Bearer {key}",
            "x-api-key": key,
            "anthropic-version": "2023-06-01",
            "Content-Type": "application/json",
        },
    )


def parse_error(body_text: str, protocol: str) -> tuple[str, str]:
    """返回 (err_type, err_msg)。解析失败时 err_type 为 '<unparsable>'。"""
    try:
        obj = json.loads(body_text)
    except Exception:
        return "<unparsable>", body_text[:400]

    if protocol == "openai":
        err = obj.get("error") if isinstance(obj, dict) else None
        if isinstance(err, dict):
            return str(err.get("type", "")), str(err.get("message", ""))
        if isinstance(err, str):
            return "error_string", err
    else:  # claude
        err = obj.get("error") if isinstance(obj, dict) else None
        if isinstance(err, dict):
            return str(err.get("type", "")), str(err.get("message", ""))
    return "", ""


def one_request(idx: int, req: urllib.request.Request, protocol: str, timeout: float) -> Result:
    t0 = time.monotonic()
    try:
        ctx = ssl.create_default_context()
        with urllib.request.urlopen(req, timeout=timeout, context=ctx) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
            elapsed = int((time.monotonic() - t0) * 1000)
            err_type, err_msg = parse_error(raw, protocol)
            ok = resp.status // 100 == 2 and not err_type
            return Result(idx, resp.status, ok, err_type, err_msg, raw[:200], elapsed)
    except urllib.error.HTTPError as e:
        raw = ""
        try:
            raw = e.read().decode("utf-8", errors="replace")
        except Exception:
            pass
        err_type, err_msg = parse_error(raw, protocol)
        return Result(idx, e.code, False, err_type, err_msg, raw[:200],
                      int((time.monotonic() - t0) * 1000))
    except Exception as e:  # URLError / timeout / ssl
        return Result(idx, 0, False, "<transport>", str(e), "",
                      int((time.monotonic() - t0) * 1000))


def run(base: str, key: str, protocol: str, model: str, prompt: str,
        n: int, concurrency: int, timeout: float) -> list[Result]:
    builder = build_openai_request if protocol == "openai" else build_claude_request
    results: list[Result] = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
        futures = [
            pool.submit(one_request, i, builder(base, key, model, prompt), protocol, timeout)
            for i in range(n)
        ]
        done = 0
        for fut in concurrent.futures.as_completed(futures):
            r = fut.result()
            results.append(r)
            done += 1
            marker = "." if r.ok else "x"
            sys.stdout.write(marker)
            if done % 50 == 0:
                sys.stdout.write(f" {done}/{n}\n")
            sys.stdout.flush()
    sys.stdout.write("\n")
    return sorted(results, key=lambda r: r.idx)


def summarize(results: list[Result]) -> None:
    total = len(results)
    ok_cnt = sum(1 for r in results if r.ok)
    fail = total - ok_cnt
    elapsed = [r.elapsed_ms for r in results]
    elapsed.sort()

    def pct(p: float) -> int:
        if not elapsed:
            return 0
        k = min(len(elapsed) - 1, int(len(elapsed) * p))
        return elapsed[k]

    status_dist = collections.Counter(r.status for r in results)
    errtype_dist = collections.Counter(r.err_type for r in results if not r.ok)
    msg_dist = collections.Counter(r.err_msg for r in results if not r.ok)

    print("\n========== 汇总 ==========")
    print(f"总请求数:     {total}")
    print(f"成功:         {ok_cnt}  ({ok_cnt / total * 100:.1f}%)")
    print(f"失败:         {fail}  ({fail / total * 100:.1f}%)")
    print(f"耗时 (ms):    p50={pct(0.50)}  p95={pct(0.95)}  p99={pct(0.99)}  "
          f"min={elapsed[0] if elapsed else 0}  max={elapsed[-1] if elapsed else 0}")

    print("\n-- HTTP 状态分布 --")
    for code, c in sorted(status_dist.items()):
        print(f"  {code:>4} : {c}")

    if errtype_dist:
        print("\n-- 失败 error.type 分布 --")
        for t, c in errtype_dist.most_common():
            print(f"  {t or '<empty>':<30} : {c}")

    if msg_dist:
        print("\n-- 失败 error.message 去重（按出现次数降序，截断 300 字）--")
        for msg, c in msg_dist.most_common(10):
            snippet = (msg[:300] + " ...") if len(msg) > 300 else msg
            print(f"  [{c:>3}x] {snippet!r}")

    default_hits = [r for r in results
                    if not r.ok and r.err_msg.lower().startswith("upstream error (status code")]
    custom_hits = [r for r in results
                   if not r.ok and "upstream error" not in r.err_msg.lower()
                   and r.err_type not in {"<transport>", "<unparsable>", "new_api_error",
                                          "invalid_request", "insufficient_user_quota"}]
    if default_hits:
        print(f"\n[探测] 有 {len(default_hits)} 条响应命中默认兜底 'upstream error (status code: N)'。")
        print("       如果你配了自定义兜底但看到这条，说明 HideUpstreamErrorMessage 可能是空字符串，"
              "或这条错误不是上游类错误（HideUpstreamDetail 对 ErrorTypeNewAPIError 直接跳过）。")
    if custom_hits:
        print(f"\n[探测] 有 {len(custom_hits)} 条响应疑似命中了自定义兜底。示例：")
        print(f"       {custom_hits[0].err_msg!r}")


def ask(prompt: str, default: str = "", *, required: bool = False,
        secret: bool = False, choices: list[str] | None = None) -> str:
    """读一行输入；空则返回 default；required=True 时不允许空。"""
    while True:
        hint = f" [{default}]" if default and not secret else ""
        if choices:
            hint = f" ({'/'.join(choices)})" + hint
        raw = input(f"{prompt}{hint}: ").strip()
        if not raw:
            if required and not default:
                print("  此项必填，请重新输入。")
                continue
            raw = default
        if choices and raw not in choices:
            print(f"  只能是 {choices} 之一。")
            continue
        return raw


def ask_int(prompt: str, default: int, *, min_v: int = 1, max_v: int = 10_000) -> int:
    while True:
        raw = input(f"{prompt} [{default}]: ").strip()
        if not raw:
            return default
        try:
            v = int(raw)
        except ValueError:
            print("  请输入整数。")
            continue
        if v < min_v or v > max_v:
            print(f"  取值范围 {min_v}~{max_v}。")
            continue
        return v


def ask_float(prompt: str, default: float, *, min_v: float = 0.1, max_v: float = 600.0) -> float:
    while True:
        raw = input(f"{prompt} [{default}]: ").strip()
        if not raw:
            return default
        try:
            v = float(raw)
        except ValueError:
            print("  请输入数字。")
            continue
        if v < min_v or v > max_v:
            print(f"  取值范围 {min_v}~{max_v}。")
            continue
        return v


BUILTIN_DEFAULTS: dict = {
    "base": "https://api.yunnet.top",
    "key": "",
    "protocol": "openai",
    "model": "gpt-4o-mini",
    "prompt": "ping, reply with 'pong' only.",
    "n": 100,
    "concurrency": 10,
    "timeout": 30.0,
    "non_interactive": False,
}


def load_config_file(path: str | None) -> dict:
    """读取 config.json 并与内置默认值合并。未指定 path 时回退到脚本同目录的 config.json。"""
    if path is None:
        path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "config.json")
    merged = dict(BUILTIN_DEFAULTS)
    if not os.path.exists(path):
        return merged
    try:
        with open(path, "r", encoding="utf-8") as f:
            file_cfg = json.load(f)
    except Exception as e:
        print(f"[warn] 读取配置文件 {path} 失败：{e}，将使用内置默认值。", file=sys.stderr)
        return merged
    if not isinstance(file_cfg, dict):
        print(f"[warn] 配置文件 {path} 顶层不是对象，忽略。", file=sys.stderr)
        return merged
    for k, v in file_cfg.items():
        if k in BUILTIN_DEFAULTS:
            merged[k] = v
        else:
            print(f"[warn] 未识别的配置项 {k!r}，已忽略。", file=sys.stderr)
    print(f"[config] 已加载 {path}")
    return merged


def validate_config(cfg: dict) -> list[str]:
    """非交互模式下校验配置。返回错误列表，空列表表示 OK。"""
    errs: list[str] = []
    if not cfg.get("base"):
        errs.append("base 不能为空")
    if not cfg.get("key"):
        errs.append("key 不能为空")
    if cfg.get("protocol") not in ("openai", "claude"):
        errs.append("protocol 必须是 openai 或 claude")
    if not cfg.get("model"):
        errs.append("model 不能为空")
    try:
        n = int(cfg.get("n", 0))
        if n < 1 or n > 100_000:
            errs.append("n 取值范围 1~100000")
    except (TypeError, ValueError):
        errs.append("n 必须是整数")
    try:
        c = int(cfg.get("concurrency", 0))
        if c < 1 or c > 500:
            errs.append("concurrency 取值范围 1~500")
    except (TypeError, ValueError):
        errs.append("concurrency 必须是整数")
    try:
        t = float(cfg.get("timeout", 0))
        if t < 0.1 or t > 600.0:
            errs.append("timeout 取值范围 0.1~600.0")
    except (TypeError, ValueError):
        errs.append("timeout 必须是数字")
    return errs


def interactive_config(defaults: dict) -> dict:
    print("=== new-api 压测脚本 ===\n直接回车使用方括号里的默认值。\n")
    base = ask("网关 base url", default=defaults["base"], required=True)
    key = ask("API key (sk-...)", default=defaults["key"], required=True,
              secret=not bool(defaults["key"]))
    protocol = ask("协议", default=defaults["protocol"], choices=["openai", "claude"])
    fallback_model = ("gpt-4o-mini" if protocol == "openai"
                      else "claude-3-5-sonnet-20241022")
    default_model = defaults.get("model") or fallback_model
    model = ask("模型名", default=default_model, required=True)
    prompt = ask("prompt 内容", default=defaults["prompt"])
    n = ask_int("请求总数 n", default=int(defaults["n"]), min_v=1, max_v=100_000)
    concurrency = ask_int("并发数", default=int(defaults["concurrency"]),
                          min_v=1, max_v=500)
    timeout = ask_float("单请求超时(秒)", default=float(defaults["timeout"]),
                        min_v=0.1, max_v=600.0)
    return {
        "base": base, "key": key, "protocol": protocol, "model": model,
        "prompt": prompt, "n": n, "concurrency": concurrency, "timeout": timeout,
    }


def main() -> None:
    parser = argparse.ArgumentParser(
        description="new-api 压测脚本（支持 config.json）",
        add_help=True,
    )
    parser.add_argument("--config", "-c", default=None,
                        help="配置文件路径，默认使用脚本同目录的 config.json")
    parser.add_argument("--non-interactive", action="store_true",
                        help="不弹交互，直接用配置里的值运行（等价于 config.json 里 non_interactive=true）")
    args = parser.parse_args()

    defaults = load_config_file(args.config)
    non_interactive = bool(defaults.get("non_interactive")) or args.non_interactive

    try:
        if non_interactive:
            cfg = {k: defaults[k] for k in (
                "base", "key", "protocol", "model", "prompt",
                "n", "concurrency", "timeout")}
            errs = validate_config(cfg)
            if errs:
                print("[error] 非交互模式配置校验失败：", file=sys.stderr)
                for e in errs:
                    print(f"  - {e}", file=sys.stderr)
                sys.exit(2)
            # 规范化类型
            cfg["n"] = int(cfg["n"])
            cfg["concurrency"] = int(cfg["concurrency"])
            cfg["timeout"] = float(cfg["timeout"])
        else:
            cfg = interactive_config(defaults)
    except (KeyboardInterrupt, EOFError):
        print("\n已取消。")
        sys.exit(130)

    print(f"\n[config] base={cfg['base']} protocol={cfg['protocol']} model={cfg['model']} "
          f"n={cfg['n']} concurrency={cfg['concurrency']} timeout={cfg['timeout']}s")
    print("开始发送 …")
    t0 = time.monotonic()
    try:
        results = run(cfg["base"], cfg["key"], cfg["protocol"], cfg["model"],
                      cfg["prompt"], cfg["n"], cfg["concurrency"], cfg["timeout"])
    except KeyboardInterrupt:
        print("\n被中断。")
        sys.exit(130)
    wall = time.monotonic() - t0
    print(f"[done] 墙上时间 {wall:.1f}s  吞吐 {len(results) / wall:.1f} req/s")
    summarize(results)


if __name__ == "__main__":
    main()
