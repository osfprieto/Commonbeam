"""Optional OpenAI Python client example; always targets the chosen local router."""

import json
import os
import urllib.request

from openai import OpenAI

base = os.environ.get("COMMONBEAM_ROUTER_URL", "http://127.0.0.1:8080")
token = os.environ.get("COMMONBEAM_SESSION_TOKEN")
if not token:
    request = urllib.request.Request(
        base + "/commonbeam/v1/session", data=b"", method="POST"
    )
    with urllib.request.urlopen(request, timeout=5) as response:
        token = json.load(response)["token"]

# Disable automatic retries: ambiguous executions must be reconciled, not duplicated.
with OpenAI(base_url=base + "/v1", api_key=token, max_retries=0) as client:
    stream = client.chat.completions.create(
        model=os.environ.get("COMMONBEAM_MODEL", "commonbeam-simulated"),
        messages=[{"role": "user", "content": "Demonstrate the local test network."}],
        max_tokens=64,
        stream=True,
    )
    for event in stream:
        if event.choices:
            print(event.choices[0].delta.content or "", end="", flush=True)
    print()
