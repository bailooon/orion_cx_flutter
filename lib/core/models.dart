enum UserChannel {
  appClaro,
  webPortal,
  whatsapp,
}

extension UserChannelLabel on UserChannel {
  String get label {
    switch (this) {
      case UserChannel.appClaro:
        return 'App Claro';
      case UserChannel.webPortal:
        return 'Web Portal';
      case UserChannel.whatsapp:
        return 'WhatsApp';
    }
  }

  String get shortLabel {
    switch (this) {
      case UserChannel.appClaro:
        return 'App';
      case UserChannel.webPortal:
        return 'Web';
      case UserChannel.whatsapp:
        return 'WhatsApp';
    }
  }
}

enum ConversationActor {
  customer,
  assistant,
  agent,
  system,
}

enum SupportCaseStatus {
  bot,
  waitingHuman,
  inProgress,
  resolved,
}

extension SupportCaseStatusLabel on SupportCaseStatus {
  String get label {
    switch (this) {
      case SupportCaseStatus.bot:
        return 'Atendimento automático';
      case SupportCaseStatus.waitingHuman:
        return 'Aguardando atendente';
      case SupportCaseStatus.inProgress:
        return 'Em atendimento';
      case SupportCaseStatus.resolved:
        return 'Concluído';
    }
  }
}

class ChatMessage {
  ChatMessage({
    required this.id,
    required this.actor,
    required this.text,
    required this.sentAt,
    required this.channel,
  });

  factory ChatMessage.fromJson(Map<String, dynamic> json) {
    return ChatMessage(
      id: json['id'] as String,
      actor: ConversationActor.values.byName(json['actor'] as String),
      text: json['text'] as String,
      sentAt: DateTime.parse(json['sentAt'] as String),
      channel: UserChannel.values.byName(json['channel'] as String),
    );
  }

  final String id;
  final ConversationActor actor;
  final String text;
  final DateTime sentAt;
  final UserChannel channel;
}

class SupportCase {
  SupportCase({
    required this.id,
    required this.customerName,
    required this.customerDocument,
    required this.planName,
    required this.intent,
    required this.intentConfidence,
    required this.summary,
    required this.status,
    required this.createdAt,
    required this.updatedAt,
    required this.messages,
    this.pendingAction,
    this.assignedAgent,
    this.hasUnreadEvent = false,
  });

  factory SupportCase.fromJson(Map<String, dynamic> json) {
    return SupportCase(
      id: json['id'] as String,
      customerName: json['customerName'] as String,
      customerDocument: json['customerDocument'] as String,
      planName: json['planName'] as String,
      intent: json['intent'] as String,
      intentConfidence: (json['intentConfidence'] as num).toDouble(),
      summary: json['summary'] as String,
      status: SupportCaseStatus.values.byName(json['status'] as String),
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
      messages: (json['messages'] as List<dynamic>)
          .map((dynamic item) =>
              ChatMessage.fromJson(item as Map<String, dynamic>))
          .toList(),
      pendingAction: json['pendingAction'] as String?,
      assignedAgent: json['assignedAgent'] as String?,
      hasUnreadEvent: json['hasUnreadEvent'] as bool? ?? false,
    );
  }

  final String id;
  final String customerName;
  final String customerDocument;
  final String planName;
  String intent;
  double intentConfidence;
  String summary;
  SupportCaseStatus status;
  final DateTime createdAt;
  DateTime updatedAt;
  final List<ChatMessage> messages;
  String? pendingAction;
  String? assignedAgent;
  bool hasUnreadEvent;

  ChatMessage? get lastMessage => messages.isEmpty ? null : messages.last;

  UserChannel get lastChannel =>
      lastMessage?.channel ?? UserChannel.appClaro;
}
