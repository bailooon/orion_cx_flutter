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
      home: RoleSelectionScreen(controller: controller),
    );
  }
}
