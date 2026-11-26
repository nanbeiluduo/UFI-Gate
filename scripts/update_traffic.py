#!/usr/bin/env python3
"""Merge GitHub traffic views and clones into site/data/traffic.json.

GitHub's traffic API only returns about 14 days, and only for a token that
can push to the repository. The workflow tries TRAFFIC_TOKEN when that secret
exists, otherwise the job's GITHUB_TOKEN. GITHUB_TOKEN often cannot read
traffic. When that happens this script keeps any history already stored and
records last_error; the website still renders stars, forks and downloads.

To enable collection, add a repository secret named TRAFFIC_TOKEN:

- Classic PAT with the repo scope, or
- Fine-grained PAT that can read repository administration (traffic) and
  write Contents on this repository.

The secret is optional. Without it the page leaves the chart empty.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "site" / "data" / "traffic.json"


def valid_date(value):
    return isinstance(value, str) and len(value) == 10 and value[4] == "-" and value[7] == "-" and value.replace("-", "").isdigit()


def merge_series(existing, rows):
    by_date = {}
    for item in existing or []:
        date = item.get("date")
        if not valid_date(date):
            continue
        by_date[date] = {
            "date": date,
            "count": int(item.get("count") or 0),
            "uniques": int(item.get("uniques") or 0),
        }
    for row in rows or []:
        timestamp = row.get("timestamp") or ""
        date = timestamp[:10]
        if not valid_date(date):
            continue
        by_date[date] = {
            "date": date,
            "count": int(row.get("count") or 0),
            "uniques": int(row.get("uniques") or 0),
        }
    return [by_date[key] for key in sorted(by_date)]


def empty_doc():
    return {
        "schema": 1,
        "updated": None,
        "available": False,
        "source": "",
        "last_error": "",
        "recent_views": None,
        "recent_uniques": None,
        "recent_clones": None,
        "recent_clone_uniques": None,
        "views": [],
        "clones": [],
    }


def load_doc():
    if not OUT.exists():
        return empty_doc()
    try:
        data = json.loads(OUT.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return empty_doc()
    base = empty_doc()
    base.update(data if isinstance(data, dict) else {})
    base["views"] = list(base.get("views") or [])
    base["clones"] = list(base.get("clones") or [])
    return base


def digest(doc):
    keys = (
        "available",
        "last_error",
        "source",
        "views",
        "clones",
        "recent_views",
        "recent_uniques",
        "recent_clones",
        "recent_clone_uniques",
    )
    return json.dumps({key: doc.get(key) for key in keys}, ensure_ascii=False, sort_keys=True)


def write_doc(doc):
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def fetch(url, token):
    request = urllib.request.Request(
        url,
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": "Bearer " + token,
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "ufi-gate-traffic",
        },
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def self_test():
    merged = merge_series(
        [{"date": "2026-01-01", "count": 1, "uniques": 1}, {"date": "bad"}],
        [
            {"timestamp": "2026-01-01T00:00:00Z", "count": 4, "uniques": 2},
            {"timestamp": "2026-01-02T00:00:00Z", "count": 3, "uniques": 3},
            {"timestamp": "", "count": 9, "uniques": 9},
        ],
    )
    assert merged == [
        {"date": "2026-01-01", "count": 4, "uniques": 2},
        {"date": "2026-01-02", "count": 3, "uniques": 3},
    ], merged
    print("self-test ok")
    return 0


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        return self_test()

    repo = os.environ.get("REPO") or "nanbeiluduo/UFI-Gate"
    traffic_token = os.environ.get("TRAFFIC_TOKEN") or ""
    github_token = os.environ.get("GITHUB_TOKEN") or ""
    token = traffic_token or github_token
    source = "traffic_token" if traffic_token else "github_token"
    doc = load_doc()
    before = digest(doc)

    if not token:
        doc["last_error"] = "没有可用的令牌，无法读取仓库流量。"
        doc["available"] = bool(doc["views"] or doc["clones"])
    else:
        try:
            views = fetch("https://api.github.com/repos/%s/traffic/views" % repo, token)
            clones = fetch("https://api.github.com/repos/%s/traffic/clones" % repo, token)
        except urllib.error.HTTPError as error:
            if error.code in (401, 403):
                hint = "GITHUB_TOKEN 通常没有流量接口权限。" if source == "github_token" else "TRAFFIC_TOKEN 被拒绝。"
                doc["last_error"] = "GitHub 流量接口返回 %s。%s请在仓库 Secrets 中设置 TRAFFIC_TOKEN（经典 PAT 的 repo 范围，或可读取 Administration 的细粒度 PAT）。" % (error.code, hint)
            else:
                doc["last_error"] = "读取流量失败：HTTP %s。" % error.code
            doc["available"] = bool(doc["views"] or doc["clones"])
            print(doc["last_error"], file=sys.stderr)
        except urllib.error.URLError as error:
            doc["last_error"] = "读取流量失败：%s" % error.reason
            doc["available"] = bool(doc["views"] or doc["clones"])
            print(doc["last_error"], file=sys.stderr)
        else:
            doc["views"] = merge_series(doc.get("views"), views.get("views"))
            doc["clones"] = merge_series(doc.get("clones"), clones.get("clones"))
            doc["available"] = True
            doc["last_error"] = ""
            doc["source"] = source
            doc["recent_views"] = int(views.get("count") or 0)
            doc["recent_uniques"] = int(views.get("uniques") or 0)
            doc["recent_clones"] = int(clones.get("count") or 0)
            doc["recent_clone_uniques"] = int(clones.get("uniques") or 0)
            print("views %s days, clones %s days, source %s" % (len(doc["views"]), len(doc["clones"]), source))

    if digest(doc) == before:
        print("traffic history unchanged")
        return 0
    doc["updated"] = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    write_doc(doc)
    print("wrote", OUT)
    return 0


if __name__ == "__main__":
    sys.exit(main())
