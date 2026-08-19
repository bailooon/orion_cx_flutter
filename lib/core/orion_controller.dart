import 'dart:async';

import 'package:flutter/foundation.dart';

import 'models.dart';

class OrionController extends ChangeNotifier {
  OrionController._() {
    _loadDemoData();
  }

  factory OrionController.demo() => OrionController._();

  static const String customerCaseId = 'CX-2026-0142';
  static const String restartAction = 'RESTART_SIGNAL';
  static const String continueAction = 'CONTINUE_PENDING_ACTION';
  static const String humanHandoffAction = 'REQUIRED_HUMAN_ASSISTANCE';

  final List<SupportCase> _cases = <SupportCase>[];
  int _messageCounter = 0;

  bool isBotTyping = false;
  bool liveAlertVisible = true;
  UserChannel currentChannel = UserChannel.appClaro;

  List<SupportCase> get cases => List<SupportCase>.unmodifiable(_cases);

  SupportCase get customerCase => caseById(customerCaseId);

  List<SupportCase> get waitingCases {
    final List<SupportCase> result = _cases
        .where((SupportCase item) =>
            item.status == SupportCaseStatus.waitingHuman)
        .toList();
    result.sort((SupportCase a, SupportCase b) =>
        a.updatedAt.compareTo(b.updatedAt));
    return result;
  }

  List<SupportCase> get activeCases => _cases
      .where((SupportCase item) =>
          item.status == SupportCaseStatus.inProgress)
      .toList();

  List<SupportCase> get resolvedCases => _cases
      .where((SupportCase item) => item.status == SupportCaseStatus.resolved)
      .toList();

  int get unattendedEvents => waitingCases
      .where((SupportCase item) => item.hasUnreadEvent)
      .length;

  SupportCase caseById(String id) {
    return _cases.firstWhere((SupportCase item) => item.id == id);
  }

  Future<void> sendCustomerMessage(String rawText) async {
    final String text = rawText.trim();
    if (text.isEmpty || isBotTyping) {
      return;
    }

    final SupportCase item = customerCase;
    _appendMessage(
      item,
      actor: ConversationActor.customer,
      text: text,
      channel: currentChannel,
    );

    if (item.status == SupportCaseStatus.waitingHuman ||
        item.status == SupportCaseStatus.inProgress) {
      item.hasUnreadEvent = true;
      item.updatedAt = DateTime.now();
      liveAlertVisible = true;
      notifyListeners();
      return;
    }

    isBotTyping = true;
    notifyListeners();
    await Future<void>.delayed(const Duration(milliseconds: 650));
    isBotTyping = false;

    final String normalized = _normalize(text);

    if (item.pendingAction == restartAction && _isAffirmative(normalized)) {
      _completeRestart(item);
      notifyListeners();
      return;
    }

    if (item.pendingAction == continueAction && _isAffirmative(normalized)) {
      _completeRestart(item);
      notifyListeners();
      return;
    }

    if (_mentionsSlowInternet(normalized)) {
      item.intent = 'SUPORTE_TECNICO';
      item.intentConfidence = 0.94;
      item.summary =
          'Cliente relata lentidão na internet. A ação sugerida é reiniciar o sinal, mantendo a sessão disponível entre canais.';
      item.pendingAction = restartAction;
      item.status = SupportCaseStatus.bot;
      _appendMessage(
        item,
        actor: ConversationActor.assistant,
        text:
            'Entendi que sua internet está lenta. Posso reiniciar o sinal da sua conexão agora?',
        channel: currentChannel,
      );
    } else if (_mentionsBillingDispute(normalized)) {
      item.intent = 'CONTESTACAO_FATURA';
      item.intentConfidence = 0.45;
      item.summary =
          'Cliente contesta uma cobrança na fatura. A confiança da classificação ficou abaixo do limite de automação e o caso requer atendimento humano.';
      item.pendingAction = humanHandoffAction;
      item.status = SupportCaseStatus.waitingHuman;
      item.hasUnreadEvent = true;
      liveAlertVisible = true;
      _appendMessage(
        item,
        actor: ConversationActor.assistant,
        text:
            'Vou transferir este atendimento para uma pessoa especialista em faturas. Seu histórico já está sendo encaminhado, então você não precisará repetir as informações.',
        channel: currentChannel,
      );
      _appendMessage(
        item,
        actor: ConversationActor.system,
        text:
            'Evento REQUIRED_HUMAN_ASSISTANCE publicado. Você entrou na fila de atendimento.',
        channel: currentChannel,
      );
    } else if (_isNegative(normalized) && item.pendingAction == restartAction) {
      item.pendingAction = null;
      _appendMessage(
        item,
        actor: ConversationActor.assistant,
        text:
            'Tudo bem. Não fiz nenhuma alteração. Posso ajudar com outro diagnóstico ou encaminhar para um atendente.',
        channel: currentChannel,
      );
    } else {
      item.intent = 'EM_ANALISE';
      item.intentConfidence = 0.68;
      item.summary =
          'Solicitação em análise pelo assistente conversacional. O cliente ainda não selecionou um fluxo específico.';
      _appendMessage(
        item,
        actor: ConversationActor.assistant,
        text:
            'Posso ajudar com internet lenta ou contestação de cobrança. Conte um pouco mais sobre o que aconteceu.',
        channel: currentChannel,
      );
    }

    item.updatedAt = DateTime.now();
    notifyListeners();
  }

  Future<void> confirmRestart() async {
    final SupportCase item = customerCase;
    if (item.pendingAction != restartAction &&
        item.pendingAction != continueAction) {
      return;
    }

    _appendMessage(
      item,
      actor: ConversationActor.customer,
      text: 'Sim, pode reiniciar.',
      channel: currentChannel,
    );
    isBotTyping = true;
    notifyListeners();
    await Future<void>.delayed(const Duration(milliseconds: 700));
    isBotTyping = false;
    _completeRestart(item);
    notifyListeners();
  }

  void declineRestart() {
    final SupportCase item = customerCase;
    if (item.pendingAction != restartAction &&
        item.pendingAction != continueAction) {
      return;
    }

    _appendMessage(
      item,
      actor: ConversationActor.customer,
      text: 'Agora não.',
      channel: currentChannel,
    );
    item.pendingAction = null;
    _appendMessage(
      item,
      actor: ConversationActor.assistant,
      text:
          'Sem problema. A ação foi cancelada e o atendimento continua disponível.',
      channel: currentChannel,
    );
    notifyListeners();
  }

  void switchChannel(UserChannel channel) {
    if (channel == currentChannel) {
      return;
    }

    final UserChannel previous = currentChannel;
    currentChannel = channel;
    final SupportCase item = customerCase;

    _appendMessage(
      item,
      actor: ConversationActor.system,
      text:
          'Sessão ${item.id} recuperada de ${previous.label} em ${channel.label}.',
      channel: channel,
    );

    if (item.pendingAction == restartAction) {
      item.pendingAction = continueAction;
      _appendMessage(
        item,
        actor: ConversationActor.assistant,
        text:
            'Encontrei uma ação pendente: reiniciar o sinal. Quer continuar por aqui?',
        channel: channel,
      );
    } else {
      _appendMessage(
        item,
        actor: ConversationActor.assistant,
        text:
            'Seu contexto foi mantido. Podemos continuar o atendimento deste ponto.',
        channel: channel,
      );
    }

    notifyListeners();
  }

  Future<void> continueHere() async {
    final SupportCase item = customerCase;
    if (item.pendingAction != continueAction) {
      return;
    }

    _appendMessage(
      item,
      actor: ConversationActor.customer,
      text: 'Sim, por favor.',
      channel: currentChannel,
    );
    isBotTyping = true;
    notifyListeners();
    await Future<void>.delayed(const Duration(milliseconds: 700));
    isBotTyping = false;
    _completeRestart(item);
    notifyListeners();
  }

  void takeCase(String caseId, {String agentName = 'Camila Rocha'}) {
    final SupportCase item = caseById(caseId);
    if (item.status != SupportCaseStatus.waitingHuman) {
      return;
    }

    item.status = SupportCaseStatus.inProgress;
    item.assignedAgent = agentName;
    item.hasUnreadEvent = false;
    item.pendingAction = null;
    item.updatedAt = DateTime.now();
    _appendMessage(
      item,
      actor: ConversationActor.system,
      text: '$agentName assumiu o atendimento.',
      channel: item.lastChannel,
    );
    notifyListeners();
  }

  void markCaseRead(String caseId) {
    final SupportCase item = caseById(caseId);
    if (!item.hasUnreadEvent) {
      return;
    }
    item.hasUnreadEvent = false;
    notifyListeners();
  }

  void sendAgentMessage(String caseId, String rawText) {
    final String text = rawText.trim();
    if (text.isEmpty) {
      return;
    }

    final SupportCase item = caseById(caseId);
    if (item.status != SupportCaseStatus.inProgress) {
      return;
    }

    _appendMessage(
      item,
      actor: ConversationActor.agent,
      text: text,
      channel: item.lastChannel,
    );
    item.hasUnreadEvent = false;
    item.updatedAt = DateTime.now();
    notifyListeners();
  }

  void resolveCase(String caseId) {
    final SupportCase item = caseById(caseId);
    if (item.status == SupportCaseStatus.resolved) {
      return;
    }

    item.status = SupportCaseStatus.resolved;
    item.pendingAction = null;
    item.hasUnreadEvent = false;
    item.updatedAt = DateTime.now();
    _appendMessage(
      item,
      actor: ConversationActor.system,
      text: 'Atendimento concluído e histórico salvo.',
      channel: item.lastChannel,
    );
    notifyListeners();
  }

  void dismissLiveAlert() {
    liveAlertVisible = false;
    notifyListeners();
  }

  void resetCustomerConversation() {
    final SupportCase item = customerCase;
    item.messages.clear();
    item.intent = 'NÃO_CLASSIFICADA';
    item.intentConfidence = 0;
    item.summary =
        'Nova sessão criada. Aguardando a primeira solicitação do cliente.';
    item.status = SupportCaseStatus.bot;
    item.pendingAction = null;
    item.assignedAgent = null;
    item.hasUnreadEvent = false;
    item.updatedAt = DateTime.now();
    currentChannel = UserChannel.appClaro;
    _appendMessage(
      item,
      actor: ConversationActor.assistant,
      text:
          'Olá! Eu sou o assistente Orion. Como posso ajudar com sua internet ou sua fatura hoje?',
      channel: currentChannel,
    );
    notifyListeners();
  }

  void resetDemo() {
    _cases.clear();
    _messageCounter = 0;
    isBotTyping = false;
    liveAlertVisible = true;
    currentChannel = UserChannel.appClaro;
    _loadDemoData();
    notifyListeners();
  }

  void _completeRestart(SupportCase item) {
    item.pendingAction = null;
    item.intent = 'SUPORTE_TECNICO';
    item.intentConfidence = 0.94;
    item.summary =
        'Cliente relatou lentidão, autorizou o reinício do sinal e recebeu a confirmação da execução.';
    item.status = SupportCaseStatus.resolved;
    item.updatedAt = DateTime.now();
    _appendMessage(
      item,
      actor: ConversationActor.assistant,
      text:
          'Pronto! O sinal foi reiniciado. Aguarde cerca de 30 segundos e teste a conexão novamente.',
      channel: currentChannel,
    );
  }

  void _appendMessage(
    SupportCase item, {
    required ConversationActor actor,
    required String text,
    required UserChannel channel,
  }) {
    _messageCounter += 1;
    item.messages.add(
      ChatMessage(
        id: 'MSG-$_messageCounter',
        actor: actor,
        text: text,
        sentAt: DateTime.now(),
        channel: channel,
      ),
    );
    item.updatedAt = DateTime.now();
  }

  bool _mentionsSlowInternet(String text) {
    return text.contains('internet') &&
        (text.contains('lenta') ||
            text.contains('lento') ||
            text.contains('lentidao') ||
            text.contains('conexao'));
  }

  bool _mentionsBillingDispute(String text) {
    return text.contains('cobranca') ||
        text.contains('indevida') ||
        text.contains('contestar') ||
        text.contains('contestacao') ||
        (text.contains('fatura') && text.contains('valor'));
  }

  bool _isAffirmative(String text) {
    return text == 'sim' ||
        text.startsWith('sim ') ||
        text.contains('pode reiniciar') ||
        text.contains('por favor');
  }

  bool _isNegative(String text) {
    return text == 'nao' ||
        text.startsWith('nao ') ||
        text.contains('agora nao');
  }

  String _normalize(String value) {
    return value
        .toLowerCase()
        .replaceAll(RegExp(r'[áàâãä]'), 'a')
        .replaceAll(RegExp(r'[éèêë]'), 'e')
        .replaceAll(RegExp(r'[íìîï]'), 'i')
        .replaceAll(RegExp(r'[óòôõö]'), 'o')
        .replaceAll(RegExp(r'[úùûü]'), 'u')
        .replaceAll('ç', 'c')
        .trim();
  }

  void _loadDemoData() {
    final DateTime now = DateTime.now();

    _cases.add(
      SupportCase(
        id: customerCaseId,
        customerName: 'Cliente Demo',
        customerDocument: '***.482.***-**',
        planName: 'Claro Pós 50 GB + Fibra',
        intent: 'NÃO_CLASSIFICADA',
        intentConfidence: 0,
        summary:
            'Nova sessão criada. Aguardando a primeira solicitação do cliente.',
        status: SupportCaseStatus.bot,
        createdAt: now.subtract(const Duration(minutes: 2)),
        updatedAt: now,
        messages: <ChatMessage>[
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.assistant,
            text:
                'Olá! Eu sou o assistente Orion. Como posso ajudar com sua internet ou sua fatura hoje?',
            sentAt: now.subtract(const Duration(minutes: 1)),
            channel: UserChannel.appClaro,
          ),
        ],
      ),
    );

    _cases.add(
      SupportCase(
        id: 'CX-2026-0139',
        customerName: 'Maria Ferreira',
        customerDocument: '***.207.***-**',
        planName: 'Claro Controle 25 GB',
        intent: 'CONTESTACAO_FATURA',
        intentConfidence: 0.45,
        summary:
            'Cliente relata cobrança indevida na última fatura. A IA classificou a intenção com 45% de confiança e solicitou transbordo humano.',
        status: SupportCaseStatus.waitingHuman,
        createdAt: now.subtract(const Duration(minutes: 11)),
        updatedAt: now.subtract(const Duration(minutes: 4)),
        pendingAction: humanHandoffAction,
        hasUnreadEvent: true,
        messages: <ChatMessage>[
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.customer,
            text: 'Quero reclamar de uma cobrança indevida na minha fatura.',
            sentAt: now.subtract(const Duration(minutes: 11)),
            channel: UserChannel.appClaro,
          ),
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.assistant,
            text:
                'Vou transferir este atendimento para uma pessoa especialista em faturas. Seu histórico será mantido.',
            sentAt: now.subtract(const Duration(minutes: 10)),
            channel: UserChannel.appClaro,
          ),
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.system,
            text:
                'Evento REQUIRED_HUMAN_ASSISTANCE recebido pela interface administrativa.',
            sentAt: now.subtract(const Duration(minutes: 10)),
            channel: UserChannel.appClaro,
          ),
        ],
      ),
    );

    _cases.add(
      SupportCase(
        id: 'CX-2026-0136',
        customerName: 'João Martins',
        customerDocument: '***.915.***-**',
        planName: 'Claro Fibra 500 Mega',
        intent: 'CONTESTACAO_FATURA',
        intentConfidence: 0.49,
        summary:
            'Cliente questiona um serviço adicional na fatura. Atendimento assumido e histórico contextualizado.',
        status: SupportCaseStatus.inProgress,
        createdAt: now.subtract(const Duration(minutes: 24)),
        updatedAt: now.subtract(const Duration(minutes: 2)),
        assignedAgent: 'Camila Rocha',
        messages: <ChatMessage>[
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.customer,
            text: 'Apareceu um serviço adicional que eu não contratei.',
            sentAt: now.subtract(const Duration(minutes: 24)),
            channel: UserChannel.webPortal,
          ),
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.assistant,
            text:
                'Vou encaminhar o caso para um atendente e manter todo o histórico.',
            sentAt: now.subtract(const Duration(minutes: 23)),
            channel: UserChannel.webPortal,
          ),
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.system,
            text: 'Camila Rocha assumiu o atendimento.',
            sentAt: now.subtract(const Duration(minutes: 6)),
            channel: UserChannel.webPortal,
          ),
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.agent,
            text:
                'Olá, João. Estou verificando a origem desse serviço adicional para você.',
            sentAt: now.subtract(const Duration(minutes: 2)),
            channel: UserChannel.webPortal,
          ),
        ],
      ),
    );

    _cases.add(
      SupportCase(
        id: 'CX-2026-0128',
        customerName: 'Paula Santos',
        customerDocument: '***.031.***-**',
        planName: 'Claro Fibra 350 Mega',
        intent: 'SUPORTE_TECNICO',
        intentConfidence: 0.96,
        summary:
            'Cliente autorizou o reinício do sinal e confirmou o restabelecimento da conexão.',
        status: SupportCaseStatus.resolved,
        createdAt: now.subtract(const Duration(hours: 2)),
        updatedAt: now.subtract(const Duration(hours: 1, minutes: 45)),
        messages: <ChatMessage>[
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.customer,
            text: 'Minha internet está lenta.',
            sentAt: now.subtract(const Duration(hours: 2)),
            channel: UserChannel.whatsapp,
          ),
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.assistant,
            text:
                'O sinal foi reiniciado. Aguarde alguns segundos e teste novamente.',
            sentAt: now.subtract(const Duration(hours: 1, minutes: 46)),
            channel: UserChannel.webPortal,
          ),
          ChatMessage(
            id: 'MSG-${++_messageCounter}',
            actor: ConversationActor.customer,
            text: 'Voltou ao normal, obrigado!',
            sentAt: now.subtract(const Duration(hours: 1, minutes: 45)),
            channel: UserChannel.webPortal,
          ),
        ],
      ),
    );
  }
}
