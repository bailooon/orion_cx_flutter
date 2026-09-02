import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:web_socket_channel/web_socket_channel.dart';

import 'models.dart';

class ApiException implements Exception {
  ApiException(this.message, {this.statusCode});

  final String message;
  final int? statusCode;

  @override
  String toString() => message;
}

/// Network surface [OrionController] depends on. Extracted so tests can inject
/// a fake implementation instead of opening a real socket.
abstract class OrionApi {
  /// Full-state frames pushed by the gateway over a single WebSocket.
  Stream<Map<String, dynamic>> snapshots();

  Future<AuthSession> login(String email, String password);
  Future<AuthSession> register({
    required String email,
    required String password,
    required String name,
    String documentMask,
    String planName,
  });

  /// Called by the controller once a session exists, so the socket and every
  /// subsequent request carry the token.
  void useToken(String? token);

  Future<Map<String, dynamic>> loadState(UserChannel channel);
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
  Future<List<Recommendation>> loadRecommendations();
  Future<void> markNotificationRead(String notificationId);
  Future<void> markAllNotificationsRead();
  Future<void> forgetMe();
  void dispose();
}

/// Talks to the ORION Gateway over REST for commands and over a single
/// WebSocket for real-time state.
///
/// Every mutation the backend applies is broadcast as a full snapshot scoped to
/// the authenticated principal, so an agent answering on the dashboard shows up
/// in the customer's chat (and vice-versa) without polling.
class ApiClient implements OrionApi {
  ApiClient({
    required this.httpBaseUrl,
    required this.wsBaseUrl,
    http.Client? httpClient,
  }) : _http = httpClient ?? http.Client();

  final String httpBaseUrl;
  final String wsBaseUrl;
  final http.Client _http;

  String? _token;
  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _channelSub;
  Timer? _reconnectTimer;
  int _reconnectAttempts = 0;
  bool _disposed = false;

  final StreamController<Map<String, dynamic>> _snapshotController =
      StreamController<Map<String, dynamic>>.broadcast();

  @override
  Stream<Map<String, dynamic>> snapshots() => _snapshotController.stream;

  @override
  void useToken(String? token) {
    if (_token == token) {
      return;
    }
    _token = token;
    // The token is part of the handshake, so the socket has to be rebuilt.
    _closeSocket();
    if (token != null) {
      _reconnectAttempts = 0;
      _ensureSocket();
    }
  }

  Map<String, String> get _headers => <String, String>{
        'Content-Type': 'application/json; charset=utf-8',
        if (_token != null) 'Authorization': 'Bearer $_token',
      };

  void _ensureSocket() {
    if (_disposed || _channel != null || _token == null) {
      return;
    }
    try {
      // Browsers cannot set headers on a WebSocket handshake, so the gateway
      // also accepts the token as a query parameter on /ws only.
      final Uri uri = Uri.parse('$wsBaseUrl?token=$_token');
      final WebSocketChannel channel = WebSocketChannel.connect(uri);
      _channel = channel;
      _channelSub = channel.stream.listen(
        (dynamic event) {
          _reconnectAttempts = 0;
          try {
            final Map<String, dynamic> payload =
                jsonDecode(event as String) as Map<String, dynamic>;
            _snapshotController.add(payload);
          } catch (error) {
            debugPrint('Orion: frame inválido ignorado: $error');
          }
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

  void _closeSocket() {
    _reconnectTimer?.cancel();
    _channelSub?.cancel();
    _channelSub = null;
    _channel?.sink.close();
    _channel = null;
  }

  /// Reconnects with a backoff capped at 30s, so a backend restart recovers by
  /// itself without hammering the server.
  void _scheduleReconnect() {
    _channelSub?.cancel();
    _channelSub = null;
    _channel = null;
    if (_disposed || _token == null) {
      return;
    }
    _reconnectAttempts++;
    final int seconds = _reconnectAttempts > 5 ? 30 : _reconnectAttempts * 2;
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(Duration(seconds: seconds), _ensureSocket);
  }

  Never _throwFor(http.Response response) {
    String message = 'Falha na comunicação com o servidor.';
    try {
      final Map<String, dynamic> body =
          jsonDecode(response.body) as Map<String, dynamic>;
      message = body['message'] as String? ?? message;
    } catch (_) {
      // The body was not the standard error envelope; keep the generic text.
    }
    throw ApiException(message, statusCode: response.statusCode);
  }

  Future<Map<String, dynamic>> _send(
    String method,
    String path, [
    Map<String, dynamic>? body,
  ]) async {
    final Uri uri = Uri.parse('$httpBaseUrl$path');
    late final http.Response response;
    try {
      if (method == 'GET') {
        response = await _http.get(uri, headers: _headers);
      } else if (method == 'DELETE') {
        response = await _http.delete(uri, headers: _headers);
      } else {
        response = await _http.post(
          uri,
          headers: _headers,
          body: jsonEncode(body ?? const <String, dynamic>{}),
        );
      }
    } on ApiException {
      rethrow;
    } catch (error) {
      throw ApiException('Não foi possível falar com o servidor Orion.');
    }

    if (response.statusCode >= 400) {
      _throwFor(response);
    }
    if (response.body.isEmpty) {
      return <String, dynamic>{};
    }
    final dynamic decoded = jsonDecode(utf8.decode(response.bodyBytes));
    if (decoded is Map<String, dynamic>) {
      return decoded;
    }
    return <String, dynamic>{'data': decoded};
  }

  Future<List<dynamic>> _sendList(String path) async {
    final Map<String, dynamic> body = await _send('GET', path);
    final dynamic data = body['data'];
    return data is List<dynamic> ? data : const <dynamic>[];
  }

  @override
  Future<AuthSession> login(String email, String password) async {
    final Map<String, dynamic> body = await _send('POST', '/api/auth/login',
        <String, dynamic>{'email': email, 'password': password});
    return AuthSession.fromJson(body);
  }

  @override
  Future<AuthSession> register({
    required String email,
    required String password,
    required String name,
    String documentMask = '',
    String planName = '',
  }) async {
    final Map<String, dynamic> body =
        await _send('POST', '/api/auth/register', <String, dynamic>{
      'email': email,
      'password': password,
      'name': name,
      'documentMask': documentMask,
      'planName': planName,
    });
    return AuthSession.fromJson(body);
  }

  @override
  Future<Map<String, dynamic>> loadState(UserChannel channel) {
    return _send('GET', '/api/state?channel=${channel.name}');
  }

  @override
  Future<void> sendCustomerMessage(
    String caseId,
    String text,
    UserChannel channel,
  ) {
    return _send('POST', '/api/cases/$caseId/messages', <String, dynamic>{
      'text': text,
      'channel': channel.name,
    });
  }

  @override
  Future<void> confirmRestart(String caseId, UserChannel channel) {
    return _send('POST', '/api/cases/$caseId/confirm-restart',
        <String, dynamic>{'channel': channel.name});
  }

  @override
  Future<void> declineRestart(String caseId, UserChannel channel) {
    return _send('POST', '/api/cases/$caseId/decline-restart',
        <String, dynamic>{'channel': channel.name});
  }

  @override
  Future<void> continueHere(String caseId, UserChannel channel) {
    return _send('POST', '/api/cases/$caseId/continue-here',
        <String, dynamic>{'channel': channel.name});
  }

  @override
  Future<void> switchChannel(
    String caseId,
    UserChannel channel,
    UserChannel previousChannel,
  ) {
    return _send('POST', '/api/cases/$caseId/switch-channel', <String, dynamic>{
      'channel': channel.name,
      'previousChannel': previousChannel.name,
    });
  }

  @override
  Future<void> takeCase(String caseId, String agentName) {
    return _send('POST', '/api/cases/$caseId/take',
        <String, dynamic>{'agentName': agentName});
  }

  @override
  Future<void> markCaseRead(String caseId) {
    return _send('POST', '/api/cases/$caseId/mark-read');
  }

  @override
  Future<void> sendAgentMessage(String caseId, String text) {
    return _send('POST', '/api/cases/$caseId/agent-messages',
        <String, dynamic>{'text': text});
  }

  @override
  Future<void> resolveCase(String caseId) {
    return _send('POST', '/api/cases/$caseId/resolve');
  }

  @override
  Future<void> resetCustomerConversation(String caseId) {
    return _send('POST', '/api/cases/$caseId/reset-conversation');
  }

  @override
  Future<void> dismissAlert() {
    return _send('POST', '/api/dismiss-alert');
  }

  @override
  Future<List<Recommendation>> loadRecommendations() async {
    final List<dynamic> raw = await _sendList('/api/recommendations');
    return raw
        .map((dynamic item) =>
            Recommendation.fromJson(item as Map<String, dynamic>))
        .toList();
  }

  @override
  Future<void> markNotificationRead(String notificationId) {
    return _send('POST', '/api/notifications/$notificationId/read');
  }

  @override
  Future<void> markAllNotificationsRead() {
    return _send('POST', '/api/notifications/read-all');
  }

  @override
  Future<void> forgetMe() {
    return _send('DELETE', '/api/auth/me');
  }

  @override
  void dispose() {
    _disposed = true;
    _closeSocket();
    _http.close();
    _snapshotController.close();
  }
}
