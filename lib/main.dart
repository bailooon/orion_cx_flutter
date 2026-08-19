import 'package:flutter/material.dart';

import 'app.dart';
import 'core/orion_controller.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(OrionCxApp(controller: OrionController.demo()));
}
