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

/// Profile of the authenticated user (RF001). The same account is valid on
/// every channel, which is what makes the journey continuous.
enum UserRole { customer, agent }

class AuthUser {
  const AuthUser({
    required this.id,
    required this.email,
    required this.name,
    required this.role,
    this.documentMask = '',
    this.planName = '',
  });

  factory AuthUser.fromJson(Map<String, dynamic> json) {
    return AuthUser(
      id: json['id'] as String,
      email: json['email'] as String? ?? '',
      name: json['name'] as String? ?? '',
      role: (json['role'] as String? ?? 'customer') == 'agent'
          ? UserRole.agent
          : UserRole.customer,
      documentMask: json['documentMask'] as String? ?? '',
      planName: json['planName'] as String? ?? '',
    );
  }

  final String id;
  final String email;
  final String name;
  final UserRole role;
  final String documentMask;
  final String planName;

  bool get isAgent => role == UserRole.agent;

  /// First name, used in greetings.
  String get shortName => name.split(' ').first;
}

/// Access token plus the profile it belongs to.
class AuthSession {
  const AuthSession({required this.token, required this.user});

  factory AuthSession.fromJson(Map<String, dynamic> json) {
    return AuthSession(
      token: json['token'] as String,
      user: AuthUser.fromJson(json['user'] as Map<String, dynamic>),
    );
  }

  final String token;
  final AuthUser user;
}

/// Lifecycle of a support ticket (RF006).
enum TicketStatus { open, inProgress, resolved }

extension TicketStatusLabel on TicketStatus {
  String get label {
    switch (this) {
      case TicketStatus.open:
        return 'Aberto';
      case TicketStatus.inProgress:
        return 'Em atendimento';
      case TicketStatus.resolved:
        return 'Concluído';
    }
  }
}

class TicketEvent {
  const TicketEvent({required this.at, required this.description});

  factory TicketEvent.fromJson(Map<String, dynamic> json) {
    return TicketEvent(
      at: DateTime.parse(json['at'] as String),
      description: json['description'] as String? ?? '',
    );
  }

  final DateTime at;
  final String description;
}

/// A protocol the customer can follow in real time.
class Ticket {
  const Ticket({
    required this.id,
    required this.conversationId,
    required this.title,
    required this.category,
    required this.status,
    required this.channel,
    required this.createdAt,
    required this.updatedAt,
    required this.timeline,
  });

  factory Ticket.fromJson(Map<String, dynamic> json) {
    return Ticket(
      id: json['id'] as String,
      conversationId: json['conversationId'] as String? ?? '',
      title: json['title'] as String? ?? '',
      category: json['category'] as String? ?? '',
      status: _ticketStatusFrom(json['status'] as String?),
      channel: _channelFrom(json['channel'] as String?),
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
      timeline: (json['timeline'] as List<dynamic>? ?? const <dynamic>[])
          .map((dynamic item) =>
              TicketEvent.fromJson(item as Map<String, dynamic>))
          .toList(),
    );
  }

  final String id;
  final String conversationId;
  final String title;
  final String category;
  final TicketStatus status;
  final UserChannel channel;
  final DateTime createdAt;
  final DateTime updatedAt;
  final List<TicketEvent> timeline;
}

/// Status change or answer pushed to the customer (RF009).
class AppNotification {
  const AppNotification({
    required this.id,
    required this.title,
    required this.body,
    required this.channel,
    required this.read,
    required this.createdAt,
  });

  factory AppNotification.fromJson(Map<String, dynamic> json) {
    return AppNotification(
      id: json['id'] as String,
      title: json['title'] as String? ?? '',
      body: json['body'] as String? ?? '',
      channel: _channelFrom(json['channel'] as String?),
      read: json['read'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }

  final String id;
  final String title;
  final String body;
  final UserChannel channel;
  final bool read;
  final DateTime createdAt;
}

/// Next-best-action derived from the customer history (RF007).
class Recommendation {
  const Recommendation({
    required this.id,
    required this.title,
    required this.body,
    required this.reason,
    required this.action,
  });

  factory Recommendation.fromJson(Map<String, dynamic> json) {
    return Recommendation(
      id: json['id'] as String,
      title: json['title'] as String? ?? '',
      body: json['body'] as String? ?? '',
      reason: json['reason'] as String? ?? '',
      action: json['action'] as String? ?? '',
    );
  }

  final String id;
  final String title;
  final String body;
  final String reason;
  final String action;
}

/// Decodes a channel, defaulting to the app so an unknown value from a future
/// backend version cannot crash the UI.
UserChannel _channelFrom(String? raw) {
  switch (raw) {
    case 'webPortal':
      return UserChannel.webPortal;
    case 'whatsapp':
      return UserChannel.whatsapp;
    default:
      return UserChannel.appClaro;
  }
}

TicketStatus _ticketStatusFrom(String? raw) {
  switch (raw) {
    case 'inProgress':
      return TicketStatus.inProgress;
    case 'resolved':
      return TicketStatus.resolved;
    default:
      return TicketStatus.open;
  }
}
