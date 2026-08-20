from __future__ import annotations

from pydantic import BaseModel

from .models import UserChannel


class CustomerMessageIn(BaseModel):
    text: str
    channel: UserChannel = UserChannel.appClaro


class ChannelActionIn(BaseModel):
    channel: UserChannel = UserChannel.appClaro


class SwitchChannelIn(BaseModel):
    channel: UserChannel
    previousChannel: UserChannel


class TakeCaseIn(BaseModel):
    agentName: str = "Camila Rocha"


class AgentMessageIn(BaseModel):
    text: str
