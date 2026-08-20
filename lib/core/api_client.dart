import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:web_socket_channel/web_socket_channel.dart';

import 'models.dart';

class ApiException implements Exception {
  ApiException(this.message);

  final String message;

  @override
  String toString() => 'ApiException: $message';
}

/// Network surface [OrionController] depends on. Extracted so tests can
/// inject a fake implementation instead of opening a real socket.
abstract class OrionApi {
  Stream<Map<String, dynamic>> snapshots();
  Future<void> sendCustomerMessage(String caseId, String text, UserChannel channel);
  Future<void> confirmRestart(String caseId, UserChannel channel);
  Future<void> declineRestart(String caseId, UserChannel channel);
  Future<void> continueHere(String caseId, UserChannel channel);
  Future<void> switchChannel(
      String caseId, UserChannel channel, UserChannel previousChannel);
  Future<void> takeCase(String caseId, String agentName);
  Future<void> markCaseRead(String caseId);
  Future<void> sendAgentMessage(String caseId, String text);
  Future<void> resolveCase(String caseId);
  Future<void> resetCustomerConversation(String caseId);
  Future<void> dismissAlert();
  Future<void> resetDemo();
  void dispose();
}

/// Talks to the Orion CX backend over REST for commands and over a single
/// WebSocket connection for real-time state (see /ws in backend/app/main.py).
/// Every mutation the backend applies is broadcast to all connected clients
/// as a full snapshot, so a customer's message shows up on the admin
/// dashboard (and vice-versa) without polling.
class ApiClient implements OrionApi {
  ApiClient({
    required this.httpBaseUrl,
    required this.wsBaseUrl,
  });

  final String httpBaseUrl;
  final String wsBaseUrl;

  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _channelSub;
  Timer? _reconnectTimer;
  bool _disposed = false;

  final StreamController<Map<String, dynamic>> _snapshotController =
      StreamController<Map<String, dynamic>>.broadcast();

  @override
  Stream<Map<String, dynamic>> snapshots() {
    _ensureSocket();
    return _snapshotController.stream;
  }

  void _ensureSocket() {
    if (_disposed || _channel != null) {
      return;
    }
    try {
      final WebSocketChannel channel =
          WebSocketChannel.connect(Uri.parse(wsBaseUrl));
      _channel = channel;
      _channelSub = channel.stream.listen(
        (dynamic event) {
          final Map<String, dynamic> payload =
              jsonDecode(event as String) as Map<String, dynamic>;
          _snapshotController.add(payload);
        },
        onError: (Object error, StackTrace stackTrace) {
          debugPrint('Orion WS error: $error');
          _scheduleReconnect();
        },
        onDone: _scheduleReconnect,
        cancelOnError: true,
      );
    } catch (error) {
      debugPrint('Orion WS connect failed: $error');
      _scheduleReconnect();
    }
  }

  void _scheduleReconnect() {
    _channelSub?.cancel();
    _channelSub = null;
    _channel = null;
    if (_disposed) {
      return;
    }
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(const Duration(seconds: 2), _ensureSocket);
  }

  Future<Map<String, dynamic>> _post(
    String path, [
    Map<String, dynamic>? body,
  ]) async {
    final http.Response response = await http.post(
      Uri.parse('$httpBaseUrl$path'),
      headers: const <String, String>{'Content-Type': 'application/json'},
      body: jsonEncode(body ?? const <String, dynamic>{}),
    );
    if (response.statusCode >= 400) {
      throw ApiException('${response.statusCode}: ${response.body}');
    }
    if (response.body.isEmpty) {
      return <String, dynamic>{};
    }
    return jsonDecode(response.body) as Map<String, dynamic>;
  }

  Future<void> sendCustomerMessage(
    String caseId,
    String text,
    UserChannel channel,
  ) {
    return _post('/api/cases/$caseId/messages', <String, dynamic>{
      'text': text,
      'channel': channel.name,
    });
  }

  Future<void> confirmRestart(String caseId, UserChannel channel) {
    return _post('/api/cases/$caseId/confirm-restart', <String, dynamic>{
      'channel': channel.name,
    });
  }

  Future<void> declineRestart(String caseId, UserChannel channel) {
    return _post('/api/cases/$caseId/decline-restart', <String, dynamic>{
      'channel': channel.name,
    });
  }

  Future<void> continueHere(String caseId, UserChannel channel) {
    return _post('/api/cases/$caseId/continue-here', <String, dynamic>{
      'channel': channel.name,
    });
  }

  Future<void> switchChannel(
    String caseId,
    UserChannel channel,
    UserChannel previousChannel,
  ) {
    return _post('/api/cases/$caseId/switch-channel', <String, dynamic>{
      'channel': channel.name,
      'previousChannel': previousChannel.name,
    });
  }

  Future<void> takeCase(String caseId, String agentName) {
    return _post('/api/cases/$caseId/take', <String, dynamic>{
      'agentName': agentName,
    });
  }

  Future<void> markCaseRead(String caseId) {
    return _post('/api/cases/$caseId/mark-read');
  }

  Future<void> sendAgentMessage(String caseId, String text) {
    return _post('/api/cases/$caseId/agent-messages', <String, dynamic>{
      'text': text,
    });
  }

  Future<void> resolveCase(String caseId) {
    return _post('/api/cases/$caseId/resolve');
  }

  Future<void> resetCustomerConversation(String caseId) {
    return _post('/api/cases/$caseId/reset-conversation');
  }

  Future<void> dismissAlert() {
    return _post('/api/dismiss-alert');
  }

  Future<void> resetDemo() {
    return _post('/api/reset');
  }

  void dispose() {
    _disposed = true;
    _reconnectTimer?.cancel();
    _channelSub?.cancel();
    _channel?.sink.close();
    _snapshotController.close();
  }
}
