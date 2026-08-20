from __future__ import annotations

import json

from fastapi import WebSocket


class ConnectionManager:
    def __init__(self) -> None:
        self.active: list[WebSocket] = []

    async def connect(self, websocket: WebSocket) -> None:
        await websocket.accept()
        self.active.append(websocket)

    def disconnect(self, websocket: WebSocket) -> None:
        if websocket in self.active:
            self.active.remove(websocket)

    async def broadcast(self, payload: dict) -> None:
        data = json.dumps(payload)
        stale: list[WebSocket] = []
        for connection in self.active:
            try:
                await connection.send_text(data)
            except Exception:
                stale.append(connection)
        for connection in stale:
            self.disconnect(connection)
