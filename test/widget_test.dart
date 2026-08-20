import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:orion_cx/app.dart';
import 'package:orion_cx/core/api_client.dart';
import 'package:orion_cx/core/models.dart';
import 'package:orion_cx/core/orion_controller.dart';

// OrionController now talks to the Orion CX backend (backend/) over HTTP and
// WebSocket instead of holding local state. These tests inject a
// [_FakeOrionApi] — which never touches the network — through
// OrionController.withApi so the UI wiring can still be exercised offline.
void main() {
  testWidgets('executa o cenário de internet lenta na área do cliente',
      (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api = _FakeOrionApi(_demoSnapshot());
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();
    expect(find.text('Escolha a experiência'), findsOneWidget);

    await tester.tap(find.text('Entrar como cliente'));
    await tester.pumpAndSettle();
    expect(find.text('Como podemos ajudar hoje?'), findsOneWidget);

    await tester.tap(find.text('Internet lenta'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.textContaining('Posso reiniciar o sinal'), findsOneWidget);
    expect(controller.customerCase.intent, 'SUPORTE_TECNICO');
  });

  testWidgets('abre o dashboard administrativo', (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api = _FakeOrionApi(_demoSnapshot());
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();

    await tester.tap(find.text('Entrar como atendente'));
    await tester.pumpAndSettle();

    expect(find.text('Fila de atendimento'), findsOneWidget);
    expect(find.textContaining('REQUIRED_HUMAN_ASSISTANCE'), findsWidgets);
  });
}

const String _customerCaseId = 'CX-2026-0142';

Map<String, dynamic> _demoSnapshot() {
  return <String, dynamic>{
    'liveAlertVisible': true,
    'cases': <Map<String, dynamic>>[
      <String, dynamic>{
        'id': _customerCaseId,
        'customerName': 'Cliente Demo',
        'customerDocument': '***.482.***-**',
        'planName': 'Claro Pós 50 GB + Fibra',
        'intent': 'NÃO_CLASSIFICADA',
        'intentConfidence': 0,
        'summary': 'Nova sessão criada. Aguardando a primeira solicitação do cliente.',
        'status': 'bot',
        'createdAt': '2026-01-01T00:00:00.000Z',
        'updatedAt': '2026-01-01T00:00:00.000Z',
        'pendingAction': null,
        'assignedAgent': null,
        'hasUnreadEvent': false,
        'messages': <Map<String, dynamic>>[
          <String, dynamic>{
            'id': 'MSG-1',
            'actor': 'assistant',
            'text': 'Olá! Eu sou o assistente Orion. Como posso ajudar com sua internet ou '
                'sua fatura hoje?',
            'sentAt': '2026-01-01T00:00:00.000Z',
            'channel': 'appClaro',
          },
        ],
      },
      <String, dynamic>{
        'id': 'CX-2026-0139',
        'customerName': 'Maria Ferreira',
        'customerDocument': '***.207.***-**',
        'planName': 'Claro Controle 25 GB',
        'intent': 'CONTESTACAO_FATURA',
        'intentConfidence': 0.45,
        'summary': 'Cliente relata cobrança indevida na última fatura.',
        'status': 'waitingHuman',
        'createdAt': '2026-01-01T00:00:00.000Z',
        'updatedAt': '2026-01-01T00:00:00.000Z',
        'pendingAction': 'REQUIRED_HUMAN_ASSISTANCE',
        'assignedAgent': null,
        'hasUnreadEvent': true,
        'messages': <Map<String, dynamic>>[
          <String, dynamic>{
            'id': 'MSG-2',
            'actor': 'system',
            'text': 'Evento REQUIRED_HUMAN_ASSISTANCE recebido pela interface administrativa.',
            'sentAt': '2026-01-01T00:00:00.000Z',
            'channel': 'appClaro',
          },
        ],
      },
    ],
  };
}

Map<String, dynamic> _slowInternetSnapshot(Map<String, dynamic> base) {
  final Map<String, dynamic> updated = Map<String, dynamic>.from(base);
  final List<dynamic> cases = List<dynamic>.from(updated['cases'] as List<dynamic>);
  final Map<String, dynamic> customerCase =
      Map<String, dynamic>.from(cases[0] as Map<String, dynamic>);
  customerCase['intent'] = 'SUPORTE_TECNICO';
  customerCase['intentConfidence'] = 0.94;
  customerCase['pendingAction'] = 'RESTART_SIGNAL';
  final List<dynamic> messages =
      List<dynamic>.from(customerCase['messages'] as List<dynamic>);
  messages.addAll(<Map<String, dynamic>>[
    <String, dynamic>{
      'id': 'MSG-3',
      'actor': 'customer',
      'text': 'Minha internet está lenta.',
      'sentAt': '2026-01-01T00:00:01.000Z',
      'channel': 'appClaro',
    },
    <String, dynamic>{
      'id': 'MSG-4',
      'actor': 'assistant',
      'text': 'Entendi que sua internet está lenta. Posso reiniciar o sinal da sua conexão '
          'agora?',
      'sentAt': '2026-01-01T00:00:02.000Z',
      'channel': 'appClaro',
    },
  ]);
  customerCase['messages'] = messages;
  cases[0] = customerCase;
  updated['cases'] = cases;
  return updated;
}

/// Fake [OrionApi] that never touches the network: it replays canned
/// snapshots through a broadcast stream, matching just enough backend
/// behaviour for the scenarios exercised in these widget tests.
class _FakeOrionApi implements OrionApi {
  _FakeOrionApi(this._initialSnapshot) {
    scheduleMicrotask(() => _controller.add(_initialSnapshot));
  }

  final Map<String, dynamic> _initialSnapshot;
  final StreamController<Map<String, dynamic>> _controller =
      StreamController<Map<String, dynamic>>.broadcast();

  @override
  Stream<Map<String, dynamic>> snapshots() => _controller.stream;

  @override
  Future<void> sendCustomerMessage(
    String caseId,
    String text,
    UserChannel channel,
  ) async {
    if (text.contains('lenta')) {
      _controller.add(_slowInternetSnapshot(_initialSnapshot));
    }
  }

  @override
  Future<void> confirmRestart(String caseId, UserChannel channel) async {}

  @override
  Future<void> declineRestart(String caseId, UserChannel channel) async {}

  @override
  Future<void> continueHere(String caseId, UserChannel channel) async {}

  @override
  Future<void> switchChannel(
    String caseId,
    UserChannel channel,
    UserChannel previousChannel,
  ) async {}

  @override
  Future<void> takeCase(String caseId, String agentName) async {}

  @override
  Future<void> markCaseRead(String caseId) async {}

  @override
  Future<void> sendAgentMessage(String caseId, String text) async {}

  @override
  Future<void> resolveCase(String caseId) async {}

  @override
  Future<void> resetCustomerConversation(String caseId) async {}

  @override
  Future<void> dismissAlert() async {}

  @override
  Future<void> resetDemo() async {}

  @override
  void dispose() {
    _controller.close();
  }
}
