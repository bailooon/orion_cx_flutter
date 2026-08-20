import 'package:flutter/material.dart';

import 'core/app_theme.dart';
import 'core/orion_controller.dart';
import 'screens/role_selection_screen.dart';

class OrionCxApp extends StatelessWidget {
  const OrionCxApp({
    required this.controller,
    super.key,
  });

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Orion CX',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light(),
      home: AnimatedBuilder(
        animation: controller,
        builder: (BuildContext context, Widget? child) {
          if (!controller.isConnected) {
            return const _ConnectingScreen();
          }
          return RoleSelectionScreen(controller: controller);
        },
      ),
    );
  }
}

class _ConnectingScreen extends StatelessWidget {
  const _ConnectingScreen();

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            CircularProgressIndicator(),
            SizedBox(height: 16),
            Text('Conectando ao servidor Orion...'),
          ],
        ),
      ),
    );
  }
}
