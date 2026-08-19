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
