/// Typed view over the `/api/admin/dashboard` payload.
///
/// Every accessor tolerates a missing or differently typed field and falls
/// back to an empty value, so a backend that adds or drops a metric never
/// crashes the dashboard.
class DashboardData {
  DashboardData(this._raw);

  factory DashboardData.fromJson(Map<String, dynamic> json) =>
      DashboardData(json);

  final Map<String, dynamic> _raw;

  Map<String, dynamic> get _analytics => _map(_raw['analytics']);
  Map<String, dynamic> get _conversations => _map(_analytics['conversations']);
  Map<String, dynamic> get _tickets => _map(_analytics['tickets']);
  Map<String, dynamic> get _satisfaction => _map(_analytics['satisfaction']);
  Map<String, dynamic> get _ai => _map(_analytics['ai']);
  Map<String, dynamic> get _security => _map(_raw['security']);

  // Operation -------------------------------------------------------------
  int get totalConversations => _int(_conversations['total']);
  int get botConversations => _int(_conversations['bot']);
  int get waitingConversations => _int(_conversations['waitingHuman']);
  int get inProgressConversations => _int(_conversations['inProgress']);
  int get resolvedConversations => _int(_conversations['resolved']);
  double get automationRate => _double(_analytics['automationRate']);
  double get handoffRate => _double(_analytics['handoffRate']);
  double get crossChannelRate => _double(_analytics['crossChannelRate']);
  double get avgConfidence => _double(_analytics['avgConfidence']);
  double get avgHandlingMinutes => _double(_analytics['avgHandlingMinutes']);
  List<BucketData> get intents => _buckets(_analytics['intents']);
  List<BucketData> get channels => _buckets(_analytics['channels']);
  int get ticketsOpen => _int(_tickets['open']);
  int get ticketsInProgress => _int(_tickets['inProgress']);
  int get ticketsResolved => _int(_tickets['resolved']);

  // Satisfaction ----------------------------------------------------------
  int get surveyResponses => _int(_satisfaction['responses']);
  double get surveyResponseRate => _double(_satisfaction['responseRate']);
  double get csatAverage => _double(_satisfaction['average']);
  double get csat => _double(_satisfaction['csat']);
  double get npsLike => _double(_satisfaction['npsLike']);
  List<int> get distribution {
    final List<dynamic> raw =
        _satisfaction['distribution'] as List<dynamic>? ?? const <dynamic>[];
    final List<int> values = raw.map(_int).toList();
    while (values.length < 5) {
      values.add(0);
    }
    return values;
  }

  List<SegmentData> get byResolver => _segments(_satisfaction['byResolver']);
  List<SegmentData> get byIntent => _segments(_satisfaction['byIntent']);
  List<SegmentData> get byAgent => _segments(_satisfaction['byAgent']);
  List<SegmentData> get byChannel => _segments(_satisfaction['byChannel']);
  List<DayData> get daily => _list(_satisfaction['daily'])
      .map((dynamic item) => DayData(_map(item)))
      .toList();
  List<SurveyData> get recentSurveys => _list(_satisfaction['recent'])
      .map((dynamic item) => SurveyData(_map(item)))
      .toList();

  // AI agents, queue and security ----------------------------------------
  String get engine => _raw['engine'] as String? ?? '';
  int get toolCalls => _int(_ai['toolCalls']);
  int get toolErrors => _int(_ai['toolErrors']);
  int get toolsBlocked => _int(_ai['toolsBlocked']);
  int get securityBlocks => _int(_ai['securityBlocks']);
  int get rateLimited => _int(_ai['rateLimited']);
  int get queueSheds => _int(_ai['queueSheds']);
  int get repliesCorrected => _int(_ai['repliesCorrected']);
  int get confirmationsRequested => _int(_ai['confirmationsRequested']);
  int get confirmationsAccepted => _int(_ai['confirmationsAccepted']);
  int get confirmationsDeclined => _int(_ai['confirmationsDeclined']);
  int get confirmationsRejected => _int(_ai['confirmationsRejected']);
  double get avgToolLatencyMs => _double(_ai['avgToolLatencyMs']);
  List<BucketData> get toolsUsage => _buckets(_ai['byTool']);
  List<BucketData> get agentsUsage => _buckets(_ai['byAgent']);
  List<AuditData> get recentAudit => _list(_ai['recent'])
      .map((dynamic item) => AuditData(_map(item)))
      .toList();

  QueueData get agentQueue => QueueData(_map(_map(_raw['queues'])['agents']));
  QueueData get nluQueue => QueueData(_map(_map(_raw['queues'])['nlu']));

  List<AgentData> get agents => _list(_raw['agents'])
      .map((dynamic item) => AgentData(_map(item)))
      .toList();

  int get rateLimitPerMinute => _int(_security['rateLimitPerMinute']);
  int get maxInputChars => _int(_security['maxInputChars']);
  int get maxToolCallsPerTurn => _int(_security['maxToolCallsPerTurn']);
  double get confirmationTtlMinutes =>
      _double(_security['confirmationTtlMinutes']);
  double get confidenceThreshold => _double(_security['confidenceThreshold']);
}

class BucketData {
  BucketData(Map<String, dynamic> raw)
      : key = raw['key'] as String? ?? '',
        count = _int(raw['count']);

  final String key;
  final int count;
}

class SegmentData {
  SegmentData(Map<String, dynamic> raw)
      : key = raw['key'] as String? ?? '',
        responses = _int(raw['responses']),
        average = _double(raw['average']),
        csat = _double(raw['csat']);

  final String key;
  final int responses;
  final double average;
  final double csat;
}

class DayData {
  DayData(Map<String, dynamic> raw)
      : day = raw['day'] as String? ?? '',
        responses = _int(raw['responses']),
        average = _double(raw['average']),
        resolved = _int(raw['resolved']);

  final String day;
  final int responses;
  final double average;
  final int resolved;

  /// "2026-10-08" → "08/10".
  String get shortLabel {
    final List<String> parts = day.split('-');
    return parts.length == 3 ? '${parts[2]}/${parts[1]}' : day;
  }
}

class SurveyData {
  SurveyData(Map<String, dynamic> raw)
      : conversationId = raw['conversationId'] as String? ?? '',
        customer = raw['customer'] as String? ?? '',
        score = _int(raw['score']),
        comment = raw['comment'] as String? ?? '',
        resolvedBy = raw['resolvedBy'] as String? ?? '',
        agent = raw['agent'] as String? ?? '',
        intent = raw['intent'] as String? ?? '';

  final String conversationId;
  final String customer;
  final int score;
  final String comment;
  final String resolvedBy;
  final String agent;
  final String intent;
}

class AuditData {
  AuditData(Map<String, dynamic> raw)
      : kind = raw['kind'] as String? ?? '',
        agent = raw['agent'] as String? ?? '',
        tool = raw['tool'] as String? ?? '',
        status = raw['status'] as String? ?? '',
        detail = raw['detail'] as String? ?? '',
        conversationId = raw['conversationId'] as String? ?? '',
        latencyMs = _int(raw['latencyMs']),
        createdAt = DateTime.tryParse(raw['createdAt'] as String? ?? '');

  final String kind;
  final String agent;
  final String tool;
  final String status;
  final String detail;
  final String conversationId;
  final int latencyMs;
  final DateTime? createdAt;

  String get kindLabel {
    switch (kind) {
      case 'tool_call':
        return 'Ferramenta';
      case 'security_block':
        return 'Bloqueio de segurança';
      case 'queue_shed':
        return 'Fila saturada';
      case 'rate_limited':
        return 'Limite de mensagens';
      case 'confirmation_requested':
        return 'Confirmação pedida';
      case 'confirmation_accepted':
        return 'Confirmação aceita';
      case 'confirmation_declined':
        return 'Confirmação recusada';
      case 'confirmation_rejected':
        return 'Double check barrou';
      case 'reply_corrected':
        return 'Resposta corrigida';
      default:
        return kind;
    }
  }
}

class QueueData {
  QueueData(Map<String, dynamic> raw)
      : available = raw.isNotEmpty,
        capacity = _int(raw['capacity']),
        maxQueue = _int(raw['maxQueue']),
        inFlight = _int(raw['inFlight']),
        queued = _int(raw['queued']),
        processed = _int(raw['processed']),
        rejected = _int(raw['rejected']),
        timedOut = _int(raw['timedOut']),
        avgWaitMs = _double(raw['avgWaitMs']),
        avgRunMs = _double(raw['avgRunMs']),
        peakInFlight = _int(raw['peakInFlight']);

  final bool available;
  final int capacity;
  final int maxQueue;
  final int inFlight;
  final int queued;
  final int processed;
  final int rejected;
  final int timedOut;
  final double avgWaitMs;
  final double avgRunMs;
  final int peakInFlight;

  double get utilization => capacity == 0 ? 0 : inFlight / capacity;
}

class AgentData {
  AgentData(Map<String, dynamic> raw)
      : id = raw['id'] as String? ?? '',
        name = raw['name'] as String? ?? '',
        tools = _list(raw['tools']).map((dynamic t) => '$t').toList();

  final String id;
  final String name;
  final List<String> tools;
}

Map<String, dynamic> _map(dynamic value) =>
    value is Map<String, dynamic> ? value : const <String, dynamic>{};

List<dynamic> _list(dynamic value) =>
    value is List<dynamic> ? value : const <dynamic>[];

int _int(dynamic value) => value is num ? value.toInt() : 0;

double _double(dynamic value) => value is num ? value.toDouble() : 0;

List<BucketData> _buckets(dynamic value) => _list(value)
    .map((dynamic item) => BucketData(_map(item)))
    .toList();

List<SegmentData> _segments(dynamic value) => _list(value)
    .map((dynamic item) => SegmentData(_map(item)))
    .toList();
