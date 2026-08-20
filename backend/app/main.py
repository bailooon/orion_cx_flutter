from __future__ import annotations

import asyncio
import json

from fastapi import FastAPI, HTTPException, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware

from .manager import ConnectionManager
from .schemas import (
    AgentMessageIn,
    ChannelActionIn,
    CustomerMessageIn,
    SwitchChannelIn,
    TakeCaseIn,
)
from .store import CaseNotFoundError, OrionStore

app = FastAPI(title="Orion CX Backend")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

store = OrionStore()
manager = ConnectionManager()

# Simulates the "bot thinking" delay from the original Flutter-only prototype.
BOT_THINK_DELAY_SECONDS = 0.65


async def broadcast_snapshot() -> None:
    await manager.broadcast({"event": "snapshot", **store.snapshot()})


def get_case_or_404(case_id: str):
    try:
        return store.get(case_id)
    except CaseNotFoundError as error:
        raise HTTPException(status_code=404, detail="Caso não encontrado") from error


@app.get("/api/cases")
def list_cases():
    return store.snapshot()


@app.get("/api/cases/{case_id}")
def get_case(case_id: str):
    return get_case_or_404(case_id).to_dict()


@app.post("/api/cases/{case_id}/messages")
async def send_customer_message(case_id: str, body: CustomerMessageIn):
    get_case_or_404(case_id)
    await asyncio.sleep(BOT_THINK_DELAY_SECONDS)
    case = store.customer_message(case_id, body.text, body.channel)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/confirm-restart")
async def confirm_restart(case_id: str, body: ChannelActionIn):
    get_case_or_404(case_id)
    await asyncio.sleep(BOT_THINK_DELAY_SECONDS)
    case = store.confirm_restart(case_id, body.channel)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/decline-restart")
async def decline_restart(case_id: str, body: ChannelActionIn):
    get_case_or_404(case_id)
    case = store.decline_restart(case_id, body.channel)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/continue-here")
async def continue_here(case_id: str, body: ChannelActionIn):
    get_case_or_404(case_id)
    await asyncio.sleep(BOT_THINK_DELAY_SECONDS)
    case = store.continue_here(case_id, body.channel)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/switch-channel")
async def switch_channel(case_id: str, body: SwitchChannelIn):
    get_case_or_404(case_id)
    case = store.switch_channel(case_id, body.channel, body.previousChannel)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/take")
async def take_case(case_id: str, body: TakeCaseIn):
    get_case_or_404(case_id)
    case = store.take_case(case_id, body.agentName)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/mark-read")
async def mark_read(case_id: str):
    get_case_or_404(case_id)
    case = store.mark_read(case_id)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/agent-messages")
async def send_agent_message(case_id: str, body: AgentMessageIn):
    get_case_or_404(case_id)
    case = store.agent_message(case_id, body.text)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/resolve")
async def resolve_case(case_id: str):
    get_case_or_404(case_id)
    case = store.resolve_case(case_id)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/cases/{case_id}/reset-conversation")
async def reset_customer_conversation(case_id: str):
    get_case_or_404(case_id)
    case = store.reset_customer_conversation(case_id)
    await broadcast_snapshot()
    return case.to_dict()


@app.post("/api/dismiss-alert")
async def dismiss_alert():
    store.dismiss_alert()
    await broadcast_snapshot()
    return store.snapshot()


@app.post("/api/reset")
async def reset_demo():
    store.reset_demo()
    await broadcast_snapshot()
    return store.snapshot()


@app.websocket("/ws")
async def ws_endpoint(websocket: WebSocket):
    await manager.connect(websocket)
    await websocket.send_text(json.dumps({"event": "snapshot", **store.snapshot()}))
    try:
        while True:
            # Clients don't need to send anything; this just keeps the
            # connection open and detects disconnects.
            await websocket.receive_text()
    except WebSocketDisconnect:
        manager.disconnect(websocket)
