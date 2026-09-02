import 'dart:async';

import 'package:flutter/foundation.dart';

import 'api_client.dart';
import 'models.dart';

/// Remote-state client for the Orion CX platform.
///
/// Commands go out over REST to the ORION Gateway; the resulting state comes
/// back through a single WebSocket as a full snapshot scoped to the
/// authenticated principal. That snapshot is what updates [cases] and triggers
/// [notifyListeners] — the HTTP response body is otherwise ignored — so the
/// customer chat and the agent dashboard always agree, even in different
/// browser tabs.
class OrionController extends ChangeNotifier {
  OrionController._(this._api) {
    _snapshotSub = _api.snapshots().listen(_applyFrame);
  }

  factory OrionController.connect({
    String? httpBaseUrl,
    String? wsBaseUrl,
  }) {
    final String resolvedHttpBaseUrl = httpBaseUrl ??
        const String.fromEnvironment(
          'ORION_API_URL',
          defaultValue: 'http://localhost:8000',
        );
    final String resolvedWsBaseUrl = wsBaseUrl ??
        const String.fromEnvironment(
          'ORION_WS_URL',
          defaultValue: 'ws://localhost:8000/ws',
        );
    return OrionController._(
      ApiClient(httpBaseUrl: resolvedHttpBaseUrl, wsBaseUrl: resolvedWsBaseUrl),
    );
  }

  /// Lets tests inject a fake [OrionApi] instead of opening a real socket.
  @visibleForTesting
  factory OrionController.withApi(OrionApi api) => OrionController._(api);

  /// Pending actions the backend can store on a conversation.
  static const String restartAction = 'RESTART_SIGNAL';
  static const String continueAction = 'CONTINUE_PENDING_ACTION';
  static const String humanHandoffAction = 'REQUIRED_HUMAN_ASSISTANCE';

  final OrionApi _api;
  late final StreamSubscription<Map<String, dynamic>> _snapshotSub;

  final Map<String, SupportCase> _casesById = <String, SupportCase>{};
  List<Ticket> _tickets = <Ticket>[];
  List<AppNotification> _notifications = <AppNotification>[];
  List<Recommendation> _recommendations = <Recommendation>[];

  AuthUser? _user;
  String? _activeCaseId;

  bool isBotTyping = false;
  bool liveAlertVisible = true;
  bool isConnected = false;
  bool isAuthenticating = false;
  String? authError;
  UserChannel currentChannel = UserChannel.appClaro;

  /// Confidence below which the gateway hands a conversation to a person.
  /// Reported by the backend so the UI never hardcodes a policy value.
  double confidenceThreshold = 0.70;

  // --- session ---------------------------------------------------------------

  AuthUser? get user => _user;
  bool get isAuthenticated => _user != null;
  bool get isAgent => _user?.isAgent ?? false;

  /// Signs in and opens the authenticated socket.
  Future<bool> login(String email, String password) async {
    return _authenticate(() => _api.login(email.trim(), password));
  }

  /// Creates a customer account and signs in with it.
  Future<bool> register({
    required String email,
    required String password,
    required String name,
    String documentMask = '',
    String planName = '',
  }) async {
    return _authenticate(() => _api.register(
          email: email.trim(),
          password: password,
          name: name.trim(),
          documentMask: documentMask,
          planName: planName,
        ));
  }

  Future<bool> _authenticate(Future<AuthSession> Function() request) async {
    isAuthenticating = true;
    authError = null;
    notifyListeners();
    try {
      final AuthSession session = await request();
      _user = session.user;
      _api.useToken(session.token);
      // The socket needs a moment to deliver the first snapshot; loading the
      // state over REST makes the first screen render immediately and, for a
      // customer, opens the conversation on the current channel.
      await refresh();
      return true;
    } on ApiException catch (error) {
      authError = error.message;
      return false;
    } catch (error) {
      authError = 'Não foi possível entrar. Tente novamente.';
      debugPrint('Orion: falha no login: $error');
      return false;
    } finally {
      isAuthenticating = false;
      notifyListeners();
    }
  }

  /// Drops the session locally. The token is not persisted anywhere, so
  /// closing the tab has the same effect.
  void logout() {
    _api.useToken(null);
    _user = null;
    _activeCaseId = null;
    _casesById.clear();
    _tickets = <Ticket>[];
    _notifications = <AppNotification>[];
    _recommendations = <Recommendation>[];
    isConnected = false;
    currentChannel = UserChannel.appClaro;
    notifyListeners();
  }

  /// Reloads the whole state over REST. Used after login and by pull-to-refresh
  /// style actions; normal updates arrive over the socket.
  Future<void> refresh() async {
    if (!isAuthenticated) {
      return;
    }
    try {
      _applyFrame(await _api.loadState(currentChannel));
      _recommendations = await _api.loadRecommendations();
      notifyListeners();
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao carregar estado: ${error.message}');
    }
  }

  /// Anonymizes the account and every trace of it (LGPD), then signs out.
  Future<bool> forgetMe() async {
    try {
      await _api.forgetMe();
      logout();
      return true;
    } on ApiException catch (error) {
      authError = error.message;
      notifyListeners();
      return false;
    }
  }

  // --- state -----------------------------------------------------------------

  List<SupportCase> get cases =>
      List<SupportCase>.unmodifiable(_casesById.values);

  List<Ticket> get tickets => List<Ticket>.unmodifiable(_tickets);

  List<AppNotification> get notifications =>
      List<AppNotification>.unmodifiable(_notifications);

  List<Recommendation> get recommendations =>
      List<Recommendation>.unmodifiable(_recommendations);

  int get unreadNotifications =>
      _notifications.where((AppNotification item) => !item.read).length;

  /// Id of the conversation the customer screens act on.
  String get customerCaseId => _activeCaseId ?? '';

  /// The customer's live conversation.
  ///
  /// Returns a placeholder while the first snapshot is in flight so the widget
  /// tree never has to null-check; screens are only reachable once
  /// [isConnected] is true.
  SupportCase get customerCase {
    final SupportCase? item = _casesById[_activeCaseId];
    if (item != null) {
      return item;
    }
    return _placeholderCase();
  }

  List<SupportCase> get waitingCases {
    final List<SupportCase> result = _casesById.values
        .where((SupportCase item) =>
            item.status == SupportCaseStatus.waitingHuman)
        .toList();
    result.sort((SupportCase a, SupportCase b) =>
        a.updatedAt.compareTo(b.updatedAt));
    return result;
  }

  List<SupportCase> get activeCases => _casesById.values
      .where((SupportCase item) => item.status == SupportCaseStatus.inProgress)
      .toList();

  List<SupportCase> get resolvedCases => _casesById.values
      .where((SupportCase item) => item.status == SupportCaseStatus.resolved)
      .toList();

  int get unattendedEvents =>
      waitingCases.where((SupportCase item) => item.hasUnreadEvent).length;

  SupportCase caseById(String id) {
    final SupportCase? item = _casesById[id];
    if (item == null) {
      throw StateError('Caso não encontrado: $id');
    }
    return item;
  }

  /// Tickets linked to one conversation, used by the customer history screen.
  List<Ticket> ticketsForCase(String caseId) => _tickets
      .where((Ticket ticket) => ticket.conversationId == caseId)
      .toList();

  SupportCase _placeholderCase() {
    final DateTime now = DateTime.now();
    return SupportCase(
      id: '—',
      customerName: _user?.name ?? 'Cliente',
      customerDocument: _user?.documentMask ?? '',
      planName: _user?.planName ?? '',
      intent: 'NAO_CLASSIFICADA',
      intentConfidence: 0,
      summary: 'Carregando sua sessão…',
      status: SupportCaseStatus.bot,
      createdAt: now,
      updatedAt: now,
      messages: <ChatMessage>[],
    );
  }

  /// Applies a snapshot frame. Frames that are not snapshots (domain events)
  /// only nudge listeners, because the snapshot that follows carries the
  /// authoritative state.
  void _applyFrame(Map<String, dynamic> payload) {
    if (payload['event'] == 'domainEvent') {
      return;
    }

    final List<dynamic> rawCases =
        payload['cases'] as List<dynamic>? ?? const <dynamic>[];
    _casesById
      ..clear()
      ..addEntries(rawCases.map((dynamic raw) {
        final SupportCase item =
            SupportCase.fromJson(raw as Map<String, dynamic>);
        return MapEntry<String, SupportCase>(item.id, item);
      }));

    final List<dynamic> rawTickets =
        payload['tickets'] as List<dynamic>? ?? const <dynamic>[];
    _tickets = rawTickets
        .map((dynamic raw) => Ticket.fromJson(raw as Map<String, dynamic>))
        .toList();

    final List<dynamic> rawNotifications =
        payload['notifications'] as List<dynamic>? ?? const <dynamic>[];
    _notifications = rawNotifications
        .map((dynamic raw) =>
            AppNotification.fromJson(raw as Map<String, dynamic>))
        .toList();

    liveAlertVisible =
        payload['liveAlertVisible'] as bool? ?? liveAlertVisible;
    confidenceThreshold =
        (payload['confidenceThreshold'] as num?)?.toDouble() ??
            confidenceThreshold;

    _refreshActiveCase();
    isConnected = true;
    notifyListeners();
  }

  /// Picks the conversation the customer screens act on: the open one, or the
  /// most recently updated if every conversation is closed.
  void _refreshActiveCase() {
    if (isAgent) {
      return;
    }
    final List<SupportCase> mine = _casesById.values.toList()
      ..sort((SupportCase a, SupportCase b) =>
          a.updatedAt.compareTo(b.updatedAt));
    if (mine.isEmpty) {
      _activeCaseId = null;
      return;
    }
    final Iterable<SupportCase> open = mine.where(
        (SupportCase item) => item.status != SupportCaseStatus.resolved);
    _activeCaseId = open.isNotEmpty ? open.last.id : mine.last.id;
  }

  // --- customer actions ------------------------------------------------------

  Future<void> sendCustomerMessage(String rawText) async {
    final String text = rawText.trim();
    if (text.isEmpty || isBotTyping || _activeCaseId == null) {
      return;
    }
    isBotTyping = true;
    notifyListeners();
    try {
      await _api.sendCustomerMessage(_activeCaseId!, text, currentChannel);
      _recommendations = await _api.loadRecommendations();
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao enviar mensagem: ${error.message}');
    } finally {
      isBotTyping = false;
      notifyListeners();
    }
  }

  Future<void> confirmRestart() async {
    final SupportCase item = customerCase;
    if (item.pendingAction != restartAction &&
        item.pendingAction != continueAction) {
      return;
    }
    await _runCustomerAction(
        () => _api.confirmRestart(_activeCaseId!, currentChannel));
  }

  Future<void> continueHere() async {
    final SupportCase item = customerCase;
    if (item.pendingAction != continueAction) {
      return;
    }
    await _runCustomerAction(
        () => _api.continueHere(_activeCaseId!, currentChannel));
  }

  Future<void> declineRestart() async {
    final SupportCase item = customerCase;
    if (item.pendingAction != restartAction &&
        item.pendingAction != continueAction) {
      return;
    }
    try {
      await _api.declineRestart(_activeCaseId!, currentChannel);
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao recusar ação: ${error.message}');
    }
  }

  Future<void> _runCustomerAction(Future<void> Function() action) async {
    if (_activeCaseId == null) {
      return;
    }
    isBotTyping = true;
    notifyListeners();
    try {
      await action();
      _recommendations = await _api.loadRecommendations();
    } on ApiException catch (error) {
      debugPrint('Orion: falha na ação do cliente: ${error.message}');
    } finally {
      isBotTyping = false;
      notifyListeners();
    }
  }

  /// Moves the customer to another channel. The backend recovers the stored
  /// context and offers to resume any pending action (RF003).
  Future<void> switchChannel(UserChannel channel) async {
    if (channel == currentChannel || _activeCaseId == null) {
      return;
    }
    final UserChannel previous = currentChannel;
    currentChannel = channel;
    notifyListeners();
    try {
      await _api.switchChannel(_activeCaseId!, channel, previous);
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao trocar de canal: ${error.message}');
    }
  }

  Future<void> resetCustomerConversation() async {
    if (_activeCaseId == null) {
      return;
    }
    currentChannel = UserChannel.appClaro;
    notifyListeners();
    try {
      await _api.resetCustomerConversation(_activeCaseId!);
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao reiniciar conversa: ${error.message}');
    }
  }

  /// Restarts the demo conversation from a clean state.
  Future<void> resetDemo() async {
    isBotTyping = false;
    liveAlertVisible = true;
    await resetCustomerConversation();
    await refresh();
  }

  Future<void> markNotificationRead(String notificationId) async {
    try {
      await _api.markNotificationRead(notificationId);
      await refresh();
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao marcar notificação: ${error.message}');
    }
  }

  Future<void> markAllNotificationsRead() async {
    try {
      await _api.markAllNotificationsRead();
      await refresh();
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao marcar notificações: ${error.message}');
    }
  }

  // --- agent actions ---------------------------------------------------------

  Future<void> takeCase(String caseId, {String agentName = ''}) async {
    final SupportCase item = caseById(caseId);
    if (item.status != SupportCaseStatus.waitingHuman) {
      return;
    }
    try {
      // The backend assigns the case to the authenticated agent; the name is
      // kept in the signature for compatibility with the existing screens.
      await _api.takeCase(caseId, agentName.isEmpty ? (_user?.name ?? '') : agentName);
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao assumir atendimento: ${error.message}');
    }
  }

  Future<void> markCaseRead(String caseId) async {
    final SupportCase item = caseById(caseId);
    if (!item.hasUnreadEvent) {
      return;
    }
    try {
      await _api.markCaseRead(caseId);
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao marcar como lido: ${error.message}');
    }
  }

  Future<void> sendAgentMessage(String caseId, String rawText) async {
    final String text = rawText.trim();
    if (text.isEmpty) {
      return;
    }
    final SupportCase item = caseById(caseId);
    if (item.status != SupportCaseStatus.inProgress) {
      return;
    }
    try {
      await _api.sendAgentMessage(caseId, text);
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao responder: ${error.message}');
    }
  }

  Future<void> resolveCase(String caseId) async {
    final SupportCase item = caseById(caseId);
    if (item.status == SupportCaseStatus.resolved) {
      return;
    }
    try {
      await _api.resolveCase(caseId);
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao concluir atendimento: ${error.message}');
    }
  }

  Future<void> dismissLiveAlert() async {
    liveAlertVisible = false;
    notifyListeners();
    try {
      await _api.dismissAlert();
    } on ApiException catch (error) {
      debugPrint('Orion: falha ao ocultar alerta: ${error.message}');
    }
  }

  @override
  void dispose() {
    _snapshotSub.cancel();
    _api.dispose();
    super.dispose();
  }
}
