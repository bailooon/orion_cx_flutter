import 'dart:async';

import 'package:flutter/foundation.dart';

import 'api_client.dart';
import 'models.dart';

/// Thin remote-state client for the Orion CX backend.
///
/// This mirrors the public API of the original in-memory demo controller so
/// that no screen needs to change: every mutating method fires an HTTP call
/// to the backend, and the resulting state comes back through a single
/// shared WebSocket ("/ws") as a full snapshot. That snapshot is what
/// actually updates [cases] and triggers [notifyListeners] — the HTTP
/// response body is otherwise ignored — so any other connected client
/// (e.g. the admin dashboard in a different browser tab) sees the same
/// update at the same time.
class OrionController extends ChangeNotifier {
  OrionController._(this._api) {
    _snapshotSub = _api.snapshots().listen(_applySnapshot);
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

  static const String customerCaseId = 'CX-2026-0142';
  static const String restartAction = 'RESTART_SIGNAL';
  static const String continueAction = 'CONTINUE_PENDING_ACTION';
  static const String humanHandoffAction = 'REQUIRED_HUMAN_ASSISTANCE';

  final OrionApi _api;
  late final StreamSubscription<Map<String, dynamic>> _snapshotSub;

  final Map<String, SupportCase> _casesById = <String, SupportCase>{};

  bool isBotTyping = false;
  bool liveAlertVisible = true;
  bool isConnected = false;
  UserChannel currentChannel = UserChannel.appClaro;

  List<SupportCase> get cases => List<SupportCase>.unmodifiable(
        _casesById.values,
      );

  SupportCase get customerCase => caseById(customerCaseId);

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

  int get unattendedEvents => waitingCases
      .where((SupportCase item) => item.hasUnreadEvent)
      .length;

  SupportCase caseById(String id) {
    final SupportCase? item = _casesById[id];
    if (item == null) {
      throw StateError('Caso não encontrado: $id');
    }
    return item;
  }

  void _applySnapshot(Map<String, dynamic> payload) {
    final List<dynamic> rawCases = payload['cases'] as List<dynamic>? ??
        const <dynamic>[];
    _casesById
      ..clear()
      ..addEntries(rawCases.map((dynamic raw) {
        final SupportCase item =
            SupportCase.fromJson(raw as Map<String, dynamic>);
        return MapEntry<String, SupportCase>(item.id, item);
      }));
    liveAlertVisible =
        payload['liveAlertVisible'] as bool? ?? liveAlertVisible;
    isConnected = true;
    notifyListeners();
  }

  Future<void> sendCustomerMessage(String rawText) async {
    final String text = rawText.trim();
    if (text.isEmpty || isBotTyping) {
      return;
    }
    isBotTyping = true;
    notifyListeners();
    try {
      await _api.sendCustomerMessage(customerCaseId, text, currentChannel);
    } catch (error) {
      debugPrint('Orion: falha ao enviar mensagem do cliente: $error');
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
    isBotTyping = true;
    notifyListeners();
    try {
      await _api.confirmRestart(customerCaseId, currentChannel);
    } catch (error) {
      debugPrint('Orion: falha ao confirmar reinício: $error');
    } finally {
      isBotTyping = false;
      notifyListeners();
    }
  }

  Future<void> declineRestart() async {
    final SupportCase item = customerCase;
    if (item.pendingAction != restartAction &&
        item.pendingAction != continueAction) {
      return;
    }
    try {
      await _api.declineRestart(customerCaseId, currentChannel);
    } catch (error) {
      debugPrint('Orion: falha ao recusar ação: $error');
    }
  }

  Future<void> switchChannel(UserChannel channel) async {
    if (channel == currentChannel) {
      return;
    }
    final UserChannel previous = currentChannel;
    currentChannel = channel;
    notifyListeners();
    try {
      await _api.switchChannel(customerCaseId, channel, previous);
    } catch (error) {
      debugPrint('Orion: falha ao trocar de canal: $error');
    }
  }

  Future<void> continueHere() async {
    final SupportCase item = customerCase;
    if (item.pendingAction != continueAction) {
      return;
    }
    isBotTyping = true;
    notifyListeners();
    try {
      await _api.continueHere(customerCaseId, currentChannel);
    } catch (error) {
      debugPrint('Orion: falha ao continuar ação pendente: $error');
    } finally {
      isBotTyping = false;
      notifyListeners();
    }
  }

  Future<void> takeCase(String caseId, {String agentName = 'Camila Rocha'}) async {
    final SupportCase item = caseById(caseId);
    if (item.status != SupportCaseStatus.waitingHuman) {
      return;
    }
    try {
      await _api.takeCase(caseId, agentName);
    } catch (error) {
      debugPrint('Orion: falha ao assumir atendimento: $error');
    }
  }

  Future<void> markCaseRead(String caseId) async {
    final SupportCase item = caseById(caseId);
    if (!item.hasUnreadEvent) {
      return;
    }
    try {
      await _api.markCaseRead(caseId);
    } catch (error) {
      debugPrint('Orion: falha ao marcar como lido: $error');
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
    } catch (error) {
      debugPrint('Orion: falha ao enviar resposta do atendente: $error');
    }
  }

  Future<void> resolveCase(String caseId) async {
    final SupportCase item = caseById(caseId);
    if (item.status == SupportCaseStatus.resolved) {
      return;
    }
    try {
      await _api.resolveCase(caseId);
    } catch (error) {
      debugPrint('Orion: falha ao concluir atendimento: $error');
    }
  }

  Future<void> dismissLiveAlert() async {
    liveAlertVisible = false;
    notifyListeners();
    try {
      await _api.dismissAlert();
    } catch (error) {
      debugPrint('Orion: falha ao ocultar alerta: $error');
    }
  }

  Future<void> resetCustomerConversation() async {
    currentChannel = UserChannel.appClaro;
    notifyListeners();
    try {
      await _api.resetCustomerConversation(customerCaseId);
    } catch (error) {
      debugPrint('Orion: falha ao reiniciar conversa: $error');
    }
  }

  Future<void> resetDemo() async {
    isBotTyping = false;
    liveAlertVisible = true;
    currentChannel = UserChannel.appClaro;
    notifyListeners();
    try {
      await _api.resetDemo();
    } catch (error) {
      debugPrint('Orion: falha ao reiniciar demonstração: $error');
    }
  }

  @override
  void dispose() {
    _snapshotSub.cancel();
    _api.dispose();
    super.dispose();
  }
}
