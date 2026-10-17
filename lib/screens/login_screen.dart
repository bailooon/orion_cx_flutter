import 'package:flutter/material.dart';

import '../core/app_theme.dart';
import '../core/orion_controller.dart';
import '../widgets/common_widgets.dart';

/// Single sign-in for every channel (RF001).
///
/// The same credentials authenticate the customer on the App, the Web Portal
/// and the WhatsApp simulator, and the token they return is what lets the
/// backend recognise the person when they switch channel mid-journey.
class LoginScreen extends StatefulWidget {
  const LoginScreen({required this.controller, super.key});

  final OrionController controller;

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final TextEditingController _email = TextEditingController();
  final TextEditingController _password = TextEditingController();
  final TextEditingController _name = TextEditingController();
  final GlobalKey<FormState> _formKey = GlobalKey<FormState>();

  bool _registering = false;

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_onControllerChanged);
    // Pre-filled with the seeded demo customer so the two acceptance flows can
    // be run without typing credentials. Both accounts are listed in the
    // README and exist only in the demo dataset.
    _email.text = 'cliente@orion.dev';
    _password.text = 'orion12345';
  }

  void _onControllerChanged() {
    if (mounted) {
      setState(() {});
    }
  }

  @override
  void dispose() {
    widget.controller.removeListener(_onControllerChanged);
    _email.dispose();
    _password.dispose();
    _name.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!(_formKey.currentState?.validate() ?? false)) {
      return;
    }
    if (_registering) {
      await widget.controller.register(
        email: _email.text,
        password: _password.text,
        name: _name.text,
        documentMask: '***.000.***-**',
        planName: 'Claro Fibra 500 Mega',
      );
    } else {
      await widget.controller.login(_email.text, _password.text);
    }
  }

  void _useAccount(String email) {
    setState(() {
      _registering = false;
      _email.text = email;
      _password.text = 'orion12345';
    });
  }

  @override
  Widget build(BuildContext context) {
    final OrionController controller = widget.controller;

    return Scaffold(
      backgroundColor: AppColors.canvas,
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 36),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 460),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  const Center(child: BrandMark()),
                  const SizedBox(height: 26),
                  Text(
                    _registering ? 'Criar conta' : 'Entrar na plataforma',
                    style: Theme.of(context).textTheme.headlineMedium,
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Uma identidade única para App, Web Portal e WhatsApp.',
                    style: Theme.of(context)
                        .textTheme
                        .bodyMedium
                        ?.copyWith(color: AppColors.muted),
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 26),
                  SurfaceCard(
                    padding: const EdgeInsets.all(20),
                    child: Form(
                      key: _formKey,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: <Widget>[
                          if (_registering) ...<Widget>[
                            TextFormField(
                              controller: _name,
                              textInputAction: TextInputAction.next,
                              decoration: const InputDecoration(
                                labelText: 'Nome completo',
                                prefixIcon: Icon(Icons.person_outline_rounded),
                              ),
                              validator: (String? value) =>
                                  (value == null || value.trim().isEmpty)
                                      ? 'Informe seu nome.'
                                      : null,
                            ),
                            const SizedBox(height: 14),
                          ],
                          TextFormField(
                            controller: _email,
                            keyboardType: TextInputType.emailAddress,
                            textInputAction: TextInputAction.next,
                            autofillHints: const <String>[AutofillHints.email],
                            decoration: const InputDecoration(
                              labelText: 'E-mail',
                              prefixIcon: Icon(Icons.alternate_email_rounded),
                            ),
                            validator: (String? value) =>
                                (value == null || !value.contains('@'))
                                    ? 'Informe um e-mail válido.'
                                    : null,
                          ),
                          const SizedBox(height: 14),
                          TextFormField(
                            controller: _password,
                            obscureText: true,
                            textInputAction: TextInputAction.done,
                            onFieldSubmitted: (_) => _submit(),
                            decoration: const InputDecoration(
                              labelText: 'Senha',
                              prefixIcon: Icon(Icons.lock_outline_rounded),
                            ),
                            validator: (String? value) =>
                                (value == null || value.length < 8)
                                    ? 'A senha deve ter ao menos 8 caracteres.'
                                    : null,
                          ),
                          if (controller.authError != null) ...<Widget>[
                            const SizedBox(height: 14),
                            Container(
                              padding: const EdgeInsets.all(12),
                              decoration: BoxDecoration(
                                color: const Color(0xFFFFF3F4),
                                borderRadius: BorderRadius.circular(10),
                                border:
                                    Border.all(color: const Color(0xFFFFD3D7)),
                              ),
                              child: Row(
                                children: <Widget>[
                                  const Icon(Icons.error_outline_rounded,
                                      color: AppColors.claroRed, size: 20),
                                  const SizedBox(width: 10),
                                  Expanded(
                                    child: Text(
                                      controller.authError!,
                                      style: Theme.of(context)
                                          .textTheme
                                          .bodySmall,
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ],
                          const SizedBox(height: 20),
                          FilledButton(
                            onPressed:
                                controller.isAuthenticating ? null : _submit,
                            child: controller.isAuthenticating
                                ? const SizedBox(
                                    height: 18,
                                    width: 18,
                                    child: CircularProgressIndicator(
                                        strokeWidth: 2, color: Colors.white),
                                  )
                                : Text(_registering ? 'Criar conta' : 'Entrar'),
                          ),
                          const SizedBox(height: 6),
                          TextButton(
                            onPressed: controller.isAuthenticating
                                ? null
                                : () => setState(() {
                                      _registering = !_registering;
                                    }),
                            child: Text(_registering
                                ? 'Já tenho conta'
                                : 'Criar uma conta de cliente'),
                          ),
                        ],
                      ),
                    ),
                  ),
                  const SizedBox(height: 18),
                  SurfaceCard(
                    color: const Color(0xFFF7F8FA),
                    padding: const EdgeInsets.all(16),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: <Widget>[
                        Text(
                          'Contas da demonstração',
                          style: Theme.of(context)
                              .textTheme
                              .titleSmall
                              ?.copyWith(fontWeight: FontWeight.w800),
                        ),
                        const SizedBox(height: 4),
                        Text(
                          'Senha: orion12345',
                          style: Theme.of(context)
                              .textTheme
                              .bodySmall
                              ?.copyWith(color: AppColors.muted),
                        ),
                        const SizedBox(height: 10),
                        Wrap(
                          spacing: 8,
                          runSpacing: 8,
                          children: <Widget>[
                            OutlinedButton.icon(
                              onPressed: () => _useAccount('cliente@orion.dev'),
                              icon: const Icon(Icons.phone_iphone_rounded,
                                  size: 16),
                              label: const Text('Cliente'),
                            ),
                            OutlinedButton.icon(
                              onPressed: () =>
                                  _useAccount('atendente@orion.dev'),
                              icon: const Icon(Icons.support_agent_rounded,
                                  size: 16),
                              label: const Text('Atendente'),
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
