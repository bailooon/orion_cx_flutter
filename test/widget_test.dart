import 'package:flutter_test/flutter_test.dart';
import 'package:orion_cx/app.dart';
import 'package:orion_cx/core/orion_controller.dart';

void main() {
  testWidgets('executa o cenário de internet lenta na área do cliente',
      (WidgetTester tester) async {
    final OrionController controller = OrionController.demo();

    await tester.pumpWidget(OrionCxApp(controller: controller));
    expect(find.text('Escolha a experiência'), findsOneWidget);

    await tester.tap(find.text('Entrar como cliente'));
    await tester.pumpAndSettle();
    expect(find.text('Como podemos ajudar hoje?'), findsOneWidget);

    await tester.tap(find.text('Internet lenta'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 750));

    expect(find.textContaining('Posso reiniciar o sinal'), findsOneWidget);
    expect(controller.customerCase.intent, 'SUPORTE_TECNICO');
  });

  testWidgets('abre o dashboard administrativo',
      (WidgetTester tester) async {
    final OrionController controller = OrionController.demo();

    await tester.pumpWidget(OrionCxApp(controller: controller));
    await tester.tap(find.text('Entrar como atendente'));
    await tester.pumpAndSettle();

    expect(find.text('Fila de atendimento'), findsOneWidget);
    expect(find.textContaining('REQUIRED_HUMAN_ASSISTANCE'), findsWidgets);
  });
}
