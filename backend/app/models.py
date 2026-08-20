from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import Enum


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


class UserChannel(str, Enum):
    appClaro = "appClaro"
    webPortal = "webPortal"
    whatsapp = "whatsapp"


CHANNEL_LABELS = {
    UserChannel.appClaro: "App Claro",
    UserChannel.webPortal: "Web Portal",
    UserChannel.whatsapp: "WhatsApp",
}


class ConversationActor(str, Enum):
    customer = "customer"
    assistant = "assistant"
    agent = "agent"
    system = "system"


class SupportCaseStatus(str, Enum):
    bot = "bot"
    waitingHuman = "waitingHuman"
    inProgress = "inProgress"
    resolved = "resolved"


@dataclass
class ChatMessage:
    id: str
    actor: ConversationActor
    text: str
    sent_at: str
    channel: UserChannel

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "actor": self.actor.value,
            "text": self.text,
            "sentAt": self.sent_at,
            "channel": self.channel.value,
        }


@dataclass
class SupportCase:
    id: str
    customer_name: str
    customer_document: str
    plan_name: str
    intent: str
    intent_confidence: float
    summary: str
    status: SupportCaseStatus
    created_at: str
    updated_at: str
    messages: list[ChatMessage] = field(default_factory=list)
    pending_action: str | None = None
    assigned_agent: str | None = None
    has_unread_event: bool = False

    @property
    def last_channel(self) -> UserChannel:
        if self.messages:
            return self.messages[-1].channel
        return UserChannel.appClaro

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "customerName": self.customer_name,
            "customerDocument": self.customer_document,
            "planName": self.plan_name,
            "intent": self.intent,
            "intentConfidence": self.intent_confidence,
            "summary": self.summary,
            "status": self.status.value,
            "createdAt": self.created_at,
            "updatedAt": self.updated_at,
            "messages": [message.to_dict() for message in self.messages],
            "pendingAction": self.pending_action,
            "assignedAgent": self.assigned_agent,
            "hasUnreadEvent": self.has_unread_event,
        }
