import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:orion_cx/app.dart';
import 'package:orion_cx/core/api_client.dart';
import 'package:orion_cx/core/models.dart';
import 'package:orion_cx/core/orion_controller.dart';

// OrionController talks to the ORION Gateway over HTTP and WebSocket instead of
// holding local state. These tests inject a [_FakeOrionApi] — which never
// touches the network — through OrionController.withApi, so the UI wiring can
// be exercised offline. The end-to-end behaviour of the platform itself is
// covered by the Go suite in backend/internal/e2e.
void main() {
  testWidgets('cliente autentica e executa o cenário de internet lenta',
      (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api = _FakeOrionApi(role: 'customer');
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();

    // Nothing is reachable before signing in.
    expect(find.text('Entrar na plataforma'), findsOneWidget);

    await tester.tap(find.text('Entrar'));
    await tester.pumpAndSettle();

    expect(find.text('Escolha a experiência'), findsOneWidget);
    expect(controller.isAuthenticated, isTrue);

    await tester.tap(find.text('Entrar como cliente'));
    await tester.pumpAndSettle();
    expect(find.text('Como podemos ajudar hoje?'), findsOneWidget);

    await tester.tap(find.text('Internet lenta'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.textContaining('Posso reiniciar o sinal'), findsOneWidget);
    expect(controller.customerCase.intent, 'SUPORTE_TECNICO');
    expect(controller.customerCase.pendingAction, 'RESTART_SIGNAL');
  });

  testWidgets('atendente abre o dashboard administrativo',
      (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api = _FakeOrionApi(role: 'agent');
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();

    await tester.tap(find.text('Entrar'));
    await tester.pumpAndSettle();

    expect(controller.isAgent, isTrue);
    // A customer area is not offered to an agent account.
    expect(find.text('Entrar como cliente'), findsNothing);

    await tester.tap(find.text('Entrar como atendente'));
    await tester.pumpAndSettle();

    expect(find.text('Fila de atendimento'), findsOneWidget);
    expect(find.textContaining('REQUIRED_HUMAN_ASSISTANCE'), findsWidgets);
  });

  testWidgets(
      'atendente vê a conversa que ainda está com a IA, com as mensagens',
      (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api = _FakeOrionApi(role: 'agent');
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();
    await tester.tap(find.text('Entrar'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Entrar como atendente'));
    await tester.pumpAndSettle();

    // A conversation the assistant is still handling is not in the human
    // queue, so it must have its own place on the dashboard.
    expect(controller.botCases, isNotEmpty);

    await tester.tap(find.text('Com a IA'));
    await tester.pumpAndSettle();

    expect(find.text('Conversas com a IA'), findsOneWidget);
    expect(find.text('Cliente Demo'), findsWidgets);
    // The agent can read the history without the customer being in the queue.
    expect(find.textContaining('Eu sou o assistente Orion'), findsWidgets);
    // And can step in instead of waiting for a handoff.
    expect(find.text('Intervir'), findsWidgets);
  });

  testWidgets('cliente vê os chamados, notificações e recomendações',
      (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api = _FakeOrionApi(role: 'customer');
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();
    await tester.tap(find.text('Entrar'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Abrir meus chamados'));
    await tester.pumpAndSettle();

    expect(find.text('Protocolos'), findsOneWidget);
    // The title shows on the ticket card and again in the notification body.
    expect(find.text('Diagnóstico de conexão'), findsWidgets);
    expect(find.text('TCK-2026-0001 • aberto em WhatsApp'), findsOneWidget);
    expect(find.text('Notificações'), findsOneWidget);
    expect(find.textContaining('Débito automático'), findsWidgets);
  });

  testWidgets('cliente responde a pesquisa de satisfação após a conclusão',
      (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api =
        _FakeOrionApi(role: 'customer', resolvedCase: true);
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();
    await tester.tap(find.text('Entrar'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Entrar como cliente'));
    await tester.pumpAndSettle();

    expect(controller.customerCase.awaitsSurvey, isTrue);
    expect(find.text('Atendimento concluído. Como foi a sua experiência?'),
        findsOneWidget);

    await tester.tap(find.byIcon(Icons.star_outline_rounded).at(4));
    await tester.pump();
    await tester.tap(find.text('Enviar avaliação'));
    await tester.pumpAndSettle();

    expect(api.submittedScore, 5);
  });

  testWidgets('atendente abre os dashboards de indicadores e satisfação',
      (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 1000));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api = _FakeOrionApi(role: 'agent');
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();
    await tester.tap(find.text('Entrar'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Entrar como atendente'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Indicadores'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.text('Indicadores da operação'), findsOneWidget);
    expect(find.text('Automação pela IA'), findsOneWidget);
    expect(find.text('Agentes de IA e fila'), findsOneWidget);

    await tester.tap(find.text('Satisfação'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.text('Satisfação do cliente'), findsOneWidget);
    expect(find.text('IA × atendimento humano'), findsOneWidget);
    expect(find.textContaining('Resolveu sem eu precisar repetir'),
        findsOneWidget);

    // Leave the dashboard so its refresh timer is cancelled.
    await tester.tap(find.text('Fila'));
    await tester.pumpAndSettle();
  });

  testWidgets('credenciais inválidas mostram o erro do backend',
      (WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    final _FakeOrionApi api = _FakeOrionApi(role: 'customer', failLogin: true);
    final OrionController controller = OrionController.withApi(api);
    addTearDown(controller.dispose);

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.pump();

    await tester.tap(find.text('Entrar'));
    await tester.pumpAndSettle();

    expect(find.text('Credenciais inválidas ou expiradas.'), findsOneWidget);
    expect(controller.isAuthenticated, isFalse);
  });
}

const String _customerCaseId = 'CX-2026-0142';
const String _queuedCaseId = 'CX-2026-0139';

Map<String, dynamic> _message(
  String id,
  String actor,
  String text, {
  String channel = 'appClaro',
  String sentAt = '2026-01-01T00:00:00.000Z',
}) {
  return <String, dynamic>{
    'id': id,
    'actor': actor,
    'text': text,
    'sentAt': sentAt,
    'channel': channel,
  };
}

Map<String, dynamic> _customerCase() {
  return <String, dynamic>{
    'id': _customerCaseId,
    'userId': 'USR-1',
    'customerName': 'Cliente Demo',
    'customerDocument': '***.482.***-**',
    'planName': 'Claro Pós 50 GB + Fibra',
    'intent': 'NAO_CLASSIFICADA',
    'intentConfidence': 0,
    'summary': 'Nova sessão criada. Aguardando a primeira solicitação do cliente.',
    'status': 'bot',
    'createdAt': '2026-01-01T00:00:00.000Z',
    'updatedAt': '2026-01-01T00:00:00.000Z',
    'pendingAction': null,
    'assignedAgent': null,
    'hasUnreadEvent': false,
    'messages': <Map<String, dynamic>>[
      _message('MSG-1', 'assistant',
          'Olá! Eu sou o assistente Orion. Como posso ajudar com sua internet ou sua fatura hoje?'),
    ],
  };
}

Map<String, dynamic> _queuedCase() {
  return <String, dynamic>{
    'id': _queuedCaseId,
    'userId': 'USR-2',
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
      _message('MSG-2', 'system',
          'Evento REQUIRED_HUMAN_ASSISTANCE recebido pela interface administrativa.'),
    ],
  };
}

Map<String, dynamic> _ticket() {
  return <String, dynamic>{
    'id': 'TCK-2026-0001',
    'userId': 'USR-1',
    'conversationId': _customerCaseId,
    'title': 'Diagnóstico de conexão',
    'category': 'SUPORTE_TECNICO',
    'status': 'open',
    'channel': 'whatsapp',
    'createdAt': '2026-01-01T00:00:00.000Z',
    'updatedAt': '2026-01-01T00:00:00.000Z',
    'timeline': <Map<String, dynamic>>[
      <String, dynamic>{
        'at': '2026-01-01T00:00:00.000Z',
        'description': 'Chamado aberto pelo canal WhatsApp.',
      },
    ],
  };
}

Map<String, dynamic> _notification() {
  return <String, dynamic>{
    'id': 'NTF-1',
    'userId': 'USR-1',
    'title': 'Chamado TCK-2026-0001 aberto',
    'body': 'Diagnóstico de conexão',
    'channel': 'whatsapp',
    'read': false,
    'createdAt': '2026-01-01T00:00:00.000Z',
  };
}

/// Snapshot scoped to a principal, exactly like the gateway builds it: a
/// customer only ever receives their own conversation.
Map<String, dynamic> _snapshotFor(String role) {
  return <String, dynamic>{
    'event': 'snapshot',
    'liveAlertVisible': true,
    'confidenceThreshold': 0.70,
    'cases': role == 'agent'
        ? <Map<String, dynamic>>[_customerCase(), _queuedCase()]
        : <Map<String, dynamic>>[_customerCase()],
    'tickets':
        role == 'agent' ? <Map<String, dynamic>>[] : <Map<String, dynamic>>[_ticket()],
    'notifications': role == 'agent'
        ? <Map<String, dynamic>>[]
        : <Map<String, dynamic>>[_notification()],
  };
}

/// Applies the state change the backend would produce for "internet lenta".
Map<String, dynamic> _slowInternetSnapshot() {
  final Map<String, dynamic> snapshot = _snapshotFor('customer');
  final List<dynamic> cases = snapshot['cases'] as List<dynamic>;
  final Map<String, dynamic> item = cases[0] as Map<String, dynamic>;
  item['intent'] = 'SUPORTE_TECNICO';
  item['intentConfidence'] = 0.94;
  item['pendingAction'] = 'RESTART_SIGNAL';
  item['updatedAt'] = '2026-01-01T00:00:02.000Z';
  (item['messages'] as List<dynamic>).addAll(<Map<String, dynamic>>[
    _message('MSG-3', 'customer', 'Minha internet está lenta.',
        sentAt: '2026-01-01T00:00:01.000Z'),
    _message(
      'MSG-4',
      'assistant',
      'Entendi que sua conexão está com problema. Posso reiniciar o sinal da sua conexão agora?',
      sentAt: '2026-01-01T00:00:02.000Z',
    ),
  ]);
  return snapshot;
}

/// Fake [OrionApi] that never touches the network: it replays canned snapshots
/// through a broadcast stream, matching just enough backend behaviour for the
/// scenarios exercised in these widget tests.
class _FakeOrionApi implements OrionApi {
  _FakeOrionApi({
    required this.role,
    this.failLogin = false,
    this.resolvedCase = false,
  });

  final String role;
  final bool failLogin;

  /// Serves the customer's conversation already resolved and unrated, so the
  /// satisfaction survey shows up.
  final bool resolvedCase;

  final StreamController<Map<String, dynamic>> _controller =
      StreamController<Map<String, dynamic>>.broadcast();

  @override
  Stream<Map<String, dynamic>> snapshots() => _controller.stream;

  @override
  Future<AuthSession> login(String email, String password) async {
    if (failLogin) {
      throw ApiException('Credenciais inválidas ou expiradas.', statusCode: 401);
    }
    return AuthSession.fromJson(<String, dynamic>{
      'token': 'fake-token',
      'user': <String, dynamic>{
        'id': role == 'agent' ? 'USR-9' : 'USR-1',
        'email': email,
        'name': role == 'agent' ? 'Camila Rocha' : 'Cliente Demo',
        'role': role,
        'documentMask': '***.482.***-**',
        'planName': 'Claro Pós 50 GB + Fibra',
      },
    });
  }

  @override
  Future<AuthSession> register({
    required String email,
    required String password,
    required String name,
    String documentMask = '',
    String planName = '',
  }) =>
      login(email, password);

  @override
  void useToken(String? token) {}

  @override
  Future<Map<String, dynamic>> loadState(UserChannel channel) async {
    final Map<String, dynamic> snapshot = _snapshotFor(role);
    if (resolvedCase) {
      final Map<String, dynamic> item =
          (snapshot['cases'] as List<dynamic>).first as Map<String, dynamic>;
      item['status'] = 'resolved';
    }
    return snapshot;
  }

  @override
  Future<void> sendCustomerMessage(
    String caseId,
    String text,
    UserChannel channel,
  ) async {
    if (text.toLowerCase().contains('lenta')) {
      _controller.add(_slowInternetSnapshot());
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
  Future<List<Recommendation>> loadRecommendations() async {
    return <Recommendation>[
      Recommendation.fromJson(<String, dynamic>{
        'id': 'REC-DEBITO-AUTOMATICO',
        'title': 'Débito automático e fatura digital',
        'body': 'Evita atrasos e mantém o histórico sempre disponível.',
        'reason': 'Você já consultou ou contestou uma cobrança.',
        'action': 'ATIVAR_DEBITO_AUTOMATICO',
      }),
    ];
  }

  @override
  Future<void> markNotificationRead(String notificationId) async {}

  @override
  Future<void> markAllNotificationsRead() async {}

  @override
  Future<void> forgetMe() async {}

  int? submittedScore;

  @override
  Future<void> submitSurvey(String caseId, int score, String comment) async {
    submittedScore = score;
  }

  @override
  Future<Map<String, dynamic>> loadDashboard() async => _dashboardPayload();

  @override
  void dispose() {
    _controller.close();
  }
}

Map<String, dynamic> _dashboardPayload() {
  return <String, dynamic>{
    'engine': 'plano-deterministico',
    'analytics': <String, dynamic>{
      'conversations': <String, dynamic>{
        'total': 6,
        'bot': 1,
        'waitingHuman': 1,
        'inProgress': 1,
        'resolved': 3,
      },
      'automationRate': 66.67,
      'handoffRate': 33.33,
      'avgConfidence': 0.81,
      'avgHandlingMinutes': 4.2,
      'crossChannelRate': 16.67,
      'intents': <dynamic>[
        <String, dynamic>{'key': 'SUPORTE_TECNICO', 'count': 3},
      ],
      'channels': <dynamic>[
        <String, dynamic>{'key': 'whatsapp', 'count': 2},
      ],
      'tickets': <String, dynamic>{'open': 1, 'inProgress': 1, 'resolved': 2},
      'satisfaction': <String, dynamic>{
        'responses': 3,
        'responseRate': 100,
        'average': 4,
        'csat': 66.67,
        'npsLike': 0,
        'distribution': <int>[0, 0, 1, 1, 1],
        'byResolver': <dynamic>[
          <String, dynamic>{
            'key': 'ia',
            'responses': 2,
            'average': 4.5,
            'csat': 100,
          },
          <String, dynamic>{
            'key': 'humano',
            'responses': 1,
            'average': 3,
            'csat': 0,
          },
        ],
        'byIntent': <dynamic>[],
        'byAgent': <dynamic>[],
        'byChannel': <dynamic>[],
        'daily': <dynamic>[
          <String, dynamic>{
            'day': '2026-10-08',
            'responses': 3,
            'average': 4,
            'resolved': 3,
          },
        ],
        'recent': <dynamic>[
          <String, dynamic>{
            'conversationId': 'CX-2026-0128',
            'customer': 'Paula',
            'score': 5,
            'comment': 'Resolveu sem eu precisar repetir nada.',
            'resolvedBy': 'ia',
            'agent': 'Assistente Orion (IA)',
            'intent': 'SUPORTE_TECNICO',
          },
        ],
      },
      'ai': <String, dynamic>{
        'toolCalls': 4,
        'securityBlocks': 1,
        'confirmationsRequested': 2,
        'confirmationsAccepted': 1,
        'byTool': <dynamic>[
          <String, dynamic>{'key': 'diagnosticar_conexao', 'count': 2},
        ],
        'byAgent': <dynamic>[],
        'recent': <dynamic>[
          <String, dynamic>{
            'kind': 'security_block',
            'status': 'bloqueada',
            'detail': 'tentativa_de_injecao_de_prompt',
            'createdAt': '2026-10-08T12:00:00Z',
          },
        ],
      },
    },
    'queues': <String, dynamic>{
      'agents': <String, dynamic>{'capacity': 8, 'maxQueue': 32, 'processed': 4},
      'nlu': <String, dynamic>{'capacity': 8, 'maxQueue': 32, 'processed': 6},
    },
    'agents': <dynamic>[
      <String, dynamic>{
        'id': 'agente_suporte_tecnico',
        'name': 'Agente de Suporte Técnico',
        'tools': <String>['diagnosticar_conexao', 'reiniciar_sinal'],
      },
    ],
    'security': <String, dynamic>{
      'rateLimitPerMinute': 30,
      'maxInputChars': 1000,
      'maxToolCallsPerTurn': 5,
      'confirmationTtlMinutes': 15,
      'confidenceThreshold': 0.7,
    },
  };
}
