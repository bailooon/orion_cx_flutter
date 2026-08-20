from __future__ import annotations

import unicodedata
from datetime import datetime, timedelta, timezone

from .models import (
    CHANNEL_LABELS,
    ChatMessage,
    ConversationActor,
    SupportCase,
    SupportCaseStatus,
    UserChannel,
    now_iso,
)

CUSTOMER_CASE_ID = "CX-2026-0142"
RESTART_ACTION = "RESTART_SIGNAL"
CONTINUE_ACTION = "CONTINUE_PENDING_ACTION"
HUMAN_HANDOFF_ACTION = "REQUIRED_HUMAN_ASSISTANCE"


def _normalize(value: str) -> str:
    value = value.lower().strip()
    decomposed = unicodedata.normalize("NFD", value)
    return "".join(ch for ch in decomposed if unicodedata.category(ch) != "Mn")


def _is_affirmative(text: str) -> bool:
    return (
        text == "sim"
        or text.startswith("sim ")
        or "pode reiniciar" in text
        or "por favor" in text
    )


def _is_negative(text: str) -> bool:
    return text == "nao" or text.startswith("nao ") or "agora nao" in text


def _mentions_slow_internet(text: str) -> bool:
    return "internet" in text and any(
        term in text for term in ("lenta", "lento", "lentidao", "conexao")
    )


def _mentions_billing_dispute(text: str) -> bool:
    return (
        "cobranca" in text
        or "indevida" in text
        or "contestar" in text
        or "contestacao" in text
        or ("fatura" in text and "valor" in text)
    )


class CaseNotFoundError(Exception):
    pass


class OrionStore:
    def __init__(self) -> None:
        self.cases: dict[str, SupportCase] = {}
        self.live_alert_visible: bool = True
        self._message_counter = 0
        self.load_demo_data()

    # -- lookups ---------------------------------------------------------
    def get(self, case_id: str) -> SupportCase:
        case = self.cases.get(case_id)
        if case is None:
            raise CaseNotFoundError(case_id)
        return case

    def snapshot(self) -> dict:
        return {
            "cases": [case.to_dict() for case in self.cases.values()],
            "liveAlertVisible": self.live_alert_visible,
        }

    # -- helpers -----------------------------------------------------------
    def _next_message_id(self) -> str:
        self._message_counter += 1
        return f"MSG-{self._message_counter}"

    def _append(
        self,
        case: SupportCase,
        actor: ConversationActor,
        text: str,
        channel: UserChannel,
    ) -> ChatMessage:
        message = ChatMessage(
            id=self._next_message_id(),
            actor=actor,
            text=text,
            sent_at=now_iso(),
            channel=channel,
        )
        case.messages.append(message)
        case.updated_at = now_iso()
        return message

    # -- customer flow -----------------------------------------------------
    def customer_message(self, case_id: str, text: str, channel: UserChannel) -> SupportCase:
        text = text.strip()
        case = self.get(case_id)
        if not text:
            return case

        self._append(case, ConversationActor.customer, text, channel)

        if case.status in (SupportCaseStatus.waitingHuman, SupportCaseStatus.inProgress):
            case.has_unread_event = True
            case.updated_at = now_iso()
            self.live_alert_visible = True
            return case

        normalized = _normalize(text)

        if case.pending_action in (RESTART_ACTION, CONTINUE_ACTION) and _is_affirmative(normalized):
            self._complete_restart(case, channel)
            return case

        if _mentions_slow_internet(normalized):
            case.intent = "SUPORTE_TECNICO"
            case.intent_confidence = 0.94
            case.summary = (
                "Cliente relata lentidão na internet. A ação sugerida é reiniciar "
                "o sinal, mantendo a sessão disponível entre canais."
            )
            case.pending_action = RESTART_ACTION
            case.status = SupportCaseStatus.bot
            self._append(
                case,
                ConversationActor.assistant,
                "Entendi que sua internet está lenta. Posso reiniciar o sinal da sua conexão agora?",
                channel,
            )
        elif _mentions_billing_dispute(normalized):
            case.intent = "CONTESTACAO_FATURA"
            case.intent_confidence = 0.45
            case.summary = (
                "Cliente contesta uma cobrança na fatura. A confiança da classificação "
                "ficou abaixo do limite de automação e o caso requer atendimento humano."
            )
            case.pending_action = HUMAN_HANDOFF_ACTION
            case.status = SupportCaseStatus.waitingHuman
            case.has_unread_event = True
            self.live_alert_visible = True
            self._append(
                case,
                ConversationActor.assistant,
                "Vou transferir este atendimento para uma pessoa especialista em faturas. "
                "Seu histórico já está sendo encaminhado, então você não precisará repetir "
                "as informações.",
                channel,
            )
            self._append(
                case,
                ConversationActor.system,
                "Evento REQUIRED_HUMAN_ASSISTANCE publicado. Você entrou na fila de atendimento.",
                channel,
            )
        elif _is_negative(normalized) and case.pending_action == RESTART_ACTION:
            case.pending_action = None
            self._append(
                case,
                ConversationActor.assistant,
                "Tudo bem. Não fiz nenhuma alteração. Posso ajudar com outro diagnóstico ou "
                "encaminhar para um atendente.",
                channel,
            )
        else:
            case.intent = "EM_ANALISE"
            case.intent_confidence = 0.68
            case.summary = (
                "Solicitação em análise pelo assistente conversacional. O cliente ainda "
                "não selecionou um fluxo específico."
            )
            self._append(
                case,
                ConversationActor.assistant,
                "Posso ajudar com internet lenta ou contestação de cobrança. Conte um pouco "
                "mais sobre o que aconteceu.",
                channel,
            )

        case.updated_at = now_iso()
        return case

    def _complete_restart(self, case: SupportCase, channel: UserChannel) -> None:
        case.pending_action = None
        case.intent = "SUPORTE_TECNICO"
        case.intent_confidence = 0.94
        case.summary = (
            "Cliente relatou lentidão, autorizou o reinício do sinal e recebeu a "
            "confirmação da execução."
        )
        case.status = SupportCaseStatus.resolved
        case.updated_at = now_iso()
        self._append(
            case,
            ConversationActor.assistant,
            "Pronto! O sinal foi reiniciado. Aguarde cerca de 30 segundos e teste a conexão "
            "novamente.",
            channel,
        )

    def confirm_restart(self, case_id: str, channel: UserChannel) -> SupportCase:
        case = self.get(case_id)
        if case.pending_action not in (RESTART_ACTION, CONTINUE_ACTION):
            return case
        self._append(case, ConversationActor.customer, "Sim, pode reiniciar.", channel)
        self._complete_restart(case, channel)
        return case

    def continue_here(self, case_id: str, channel: UserChannel) -> SupportCase:
        case = self.get(case_id)
        if case.pending_action != CONTINUE_ACTION:
            return case
        self._append(case, ConversationActor.customer, "Sim, por favor.", channel)
        self._complete_restart(case, channel)
        return case

    def decline_restart(self, case_id: str, channel: UserChannel) -> SupportCase:
        case = self.get(case_id)
        if case.pending_action not in (RESTART_ACTION, CONTINUE_ACTION):
            return case
        self._append(case, ConversationActor.customer, "Agora não.", channel)
        case.pending_action = None
        self._append(
            case,
            ConversationActor.assistant,
            "Sem problema. A ação foi cancelada e o atendimento continua disponível.",
            channel,
        )
        return case

    def switch_channel(
        self,
        case_id: str,
        new_channel: UserChannel,
        previous_channel: UserChannel,
    ) -> SupportCase:
        case = self.get(case_id)
        self._append(
            case,
            ConversationActor.system,
            f"Sessão {case.id} recuperada de {CHANNEL_LABELS[previous_channel]} em "
            f"{CHANNEL_LABELS[new_channel]}.",
            new_channel,
        )
        if case.pending_action == RESTART_ACTION:
            case.pending_action = CONTINUE_ACTION
            self._append(
                case,
                ConversationActor.assistant,
                "Encontrei uma ação pendente: reiniciar o sinal. Quer continuar por aqui?",
                new_channel,
            )
        else:
            self._append(
                case,
                ConversationActor.assistant,
                "Seu contexto foi mantido. Podemos continuar o atendimento deste ponto.",
                new_channel,
            )
        return case

    def reset_customer_conversation(self, case_id: str) -> SupportCase:
        case = self.get(case_id)
        case.messages.clear()
        case.intent = "NÃO_CLASSIFICADA"
        case.intent_confidence = 0
        case.summary = "Nova sessão criada. Aguardando a primeira solicitação do cliente."
        case.status = SupportCaseStatus.bot
        case.pending_action = None
        case.assigned_agent = None
        case.has_unread_event = False
        case.updated_at = now_iso()
        self._append(
            case,
            ConversationActor.assistant,
            "Olá! Eu sou o assistente Orion. Como posso ajudar com sua internet ou sua fatura hoje?",
            UserChannel.appClaro,
        )
        return case

    # -- admin flow ----------------------------------------------------------
    def take_case(self, case_id: str, agent_name: str) -> SupportCase:
        case = self.get(case_id)
        if case.status != SupportCaseStatus.waitingHuman:
            return case
        case.status = SupportCaseStatus.inProgress
        case.assigned_agent = agent_name
        case.has_unread_event = False
        case.pending_action = None
        case.updated_at = now_iso()
        self._append(
            case,
            ConversationActor.system,
            f"{agent_name} assumiu o atendimento.",
            case.last_channel,
        )
        return case

    def mark_read(self, case_id: str) -> SupportCase:
        case = self.get(case_id)
        case.has_unread_event = False
        return case

    def agent_message(self, case_id: str, text: str) -> SupportCase:
        text = text.strip()
        case = self.get(case_id)
        if not text or case.status != SupportCaseStatus.inProgress:
            return case
        self._append(case, ConversationActor.agent, text, case.last_channel)
        case.has_unread_event = False
        case.updated_at = now_iso()
        return case

    def resolve_case(self, case_id: str) -> SupportCase:
        case = self.get(case_id)
        if case.status == SupportCaseStatus.resolved:
            return case
        case.status = SupportCaseStatus.resolved
        case.pending_action = None
        case.has_unread_event = False
        case.updated_at = now_iso()
        self._append(
            case,
            ConversationActor.system,
            "Atendimento concluído e histórico salvo.",
            case.last_channel,
        )
        return case

    def dismiss_alert(self) -> None:
        self.live_alert_visible = False

    def reset_demo(self) -> None:
        self.cases.clear()
        self._message_counter = 0
        self.live_alert_visible = True
        self.load_demo_data()

    # -- seed data -------------------------------------------------------------
    def load_demo_data(self) -> None:
        now = datetime.now(timezone.utc)

        def iso(delta: timedelta) -> str:
            return (now - delta).isoformat()

        case1 = SupportCase(
            id=CUSTOMER_CASE_ID,
            customer_name="Cliente Demo",
            customer_document="***.482.***-**",
            plan_name="Claro Pós 50 GB + Fibra",
            intent="NÃO_CLASSIFICADA",
            intent_confidence=0,
            summary="Nova sessão criada. Aguardando a primeira solicitação do cliente.",
            status=SupportCaseStatus.bot,
            created_at=iso(timedelta(minutes=2)),
            updated_at=iso(timedelta(0)),
            messages=[
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.assistant,
                    text="Olá! Eu sou o assistente Orion. Como posso ajudar com sua internet "
                    "ou sua fatura hoje?",
                    sent_at=iso(timedelta(minutes=1)),
                    channel=UserChannel.appClaro,
                ),
            ],
        )

        case2 = SupportCase(
            id="CX-2026-0139",
            customer_name="Maria Ferreira",
            customer_document="***.207.***-**",
            plan_name="Claro Controle 25 GB",
            intent="CONTESTACAO_FATURA",
            intent_confidence=0.45,
            summary="Cliente relata cobrança indevida na última fatura. A IA classificou a "
            "intenção com 45% de confiança e solicitou transbordo humano.",
            status=SupportCaseStatus.waitingHuman,
            created_at=iso(timedelta(minutes=11)),
            updated_at=iso(timedelta(minutes=4)),
            pending_action=HUMAN_HANDOFF_ACTION,
            has_unread_event=True,
            messages=[
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.customer,
                    text="Quero reclamar de uma cobrança indevida na minha fatura.",
                    sent_at=iso(timedelta(minutes=11)),
                    channel=UserChannel.appClaro,
                ),
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.assistant,
                    text="Vou transferir este atendimento para uma pessoa especialista em "
                    "faturas. Seu histórico será mantido.",
                    sent_at=iso(timedelta(minutes=10)),
                    channel=UserChannel.appClaro,
                ),
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.system,
                    text="Evento REQUIRED_HUMAN_ASSISTANCE recebido pela interface administrativa.",
                    sent_at=iso(timedelta(minutes=10)),
                    channel=UserChannel.appClaro,
                ),
            ],
        )

        case3 = SupportCase(
            id="CX-2026-0136",
            customer_name="João Martins",
            customer_document="***.915.***-**",
            plan_name="Claro Fibra 500 Mega",
            intent="CONTESTACAO_FATURA",
            intent_confidence=0.49,
            summary="Cliente questiona um serviço adicional na fatura. Atendimento assumido "
            "e histórico contextualizado.",
            status=SupportCaseStatus.inProgress,
            created_at=iso(timedelta(minutes=24)),
            updated_at=iso(timedelta(minutes=2)),
            assigned_agent="Camila Rocha",
            messages=[
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.customer,
                    text="Apareceu um serviço adicional que eu não contratei.",
                    sent_at=iso(timedelta(minutes=24)),
                    channel=UserChannel.webPortal,
                ),
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.assistant,
                    text="Vou encaminhar o caso para um atendente e manter todo o histórico.",
                    sent_at=iso(timedelta(minutes=23)),
                    channel=UserChannel.webPortal,
                ),
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.system,
                    text="Camila Rocha assumiu o atendimento.",
                    sent_at=iso(timedelta(minutes=6)),
                    channel=UserChannel.webPortal,
                ),
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.agent,
                    text="Olá, João. Estou verificando a origem desse serviço adicional para você.",
                    sent_at=iso(timedelta(minutes=2)),
                    channel=UserChannel.webPortal,
                ),
            ],
        )

        case4 = SupportCase(
            id="CX-2026-0128",
            customer_name="Paula Santos",
            customer_document="***.031.***-**",
            plan_name="Claro Fibra 350 Mega",
            intent="SUPORTE_TECNICO",
            intent_confidence=0.96,
            summary="Cliente autorizou o reinício do sinal e confirmou o restabelecimento "
            "da conexão.",
            status=SupportCaseStatus.resolved,
            created_at=iso(timedelta(hours=2)),
            updated_at=iso(timedelta(hours=1, minutes=45)),
            messages=[
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.customer,
                    text="Minha internet está lenta.",
                    sent_at=iso(timedelta(hours=2)),
                    channel=UserChannel.whatsapp,
                ),
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.assistant,
                    text="O sinal foi reiniciado. Aguarde alguns segundos e teste novamente.",
                    sent_at=iso(timedelta(hours=1, minutes=46)),
                    channel=UserChannel.webPortal,
                ),
                ChatMessage(
                    id=self._next_message_id(),
                    actor=ConversationActor.customer,
                    text="Voltou ao normal, obrigado!",
                    sent_at=iso(timedelta(hours=1, minutes=45)),
                    channel=UserChannel.webPortal,
                ),
            ],
        )

        for case in (case1, case2, case3, case4):
            self.cases[case.id] = case
