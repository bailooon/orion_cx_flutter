import 'package:flutter/material.dart';

import '../core/app_theme.dart';
import '../core/orion_controller.dart';
import '../widgets/common_widgets.dart';
import 'admin_portal.dart';
import 'customer_portal.dart';

class RoleSelectionScreen extends StatelessWidget {
  const RoleSelectionScreen({
    required this.controller,
    super.key,
  });

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: LayoutBuilder(
        builder: (BuildContext context, BoxConstraints constraints) {
          final bool wide = constraints.maxWidth >= 920;
          if (wide) {
            return Row(
              children: <Widget>[
                Expanded(
                  flex: 11,
                  child: _HeroPanel(controller: controller),
                ),
                Expanded(
                  flex: 9,
                  child: _RolePanel(controller: controller),
                ),
              ],
            );
          }

          return SingleChildScrollView(
            child: Column(
              children: <Widget>[
                SizedBox(
                  height: constraints.maxWidth < 500 ? 720 : 620,
                  child: _HeroPanel(controller: controller),
                ),
                _RolePanel(controller: controller),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _HeroPanel extends StatelessWidget {
  const _HeroPanel({required this.controller});

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return Container(
      color: AppColors.ink,
      child: Stack(
        fit: StackFit.expand,
        children: <Widget>[
          const CustomPaint(painter: _ArchitecturePainter()),
          Container(
            decoration: const BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: <Color>[
                  Color(0xF20D0D10),
                  Color(0xE61F0C10),
                  Color(0xF217171B),
                ],
              ),
            ),
          ),
          SafeArea(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(38, 34, 38, 38),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: <Widget>[
                  const BrandMark(onDark: true),
                  const Spacer(),
                  Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 11, vertical: 7),
                    decoration: BoxDecoration(
                      color: Colors.white.withValues(alpha: 0.08),
                      borderRadius: BorderRadius.circular(999),
                      border: Border.all(color: Colors.white24),
                    ),
                    child: const Text(
                      'PROTÓTIPO FUNCIONAL • SPRINT 2',
                      style: TextStyle(
                        color: Colors.white,
                        fontSize: 11,
                        fontWeight: FontWeight.w800,
                        letterSpacing: 0.8,
                      ),
                    ),
                  ),
                  const SizedBox(height: 18),
                  Text(
                    'Um atendimento que continua, mesmo quando o canal muda.',
                    style: Theme.of(context).textTheme.headlineLarge?.copyWith(
                          color: Colors.white,
                          fontSize: 46,
                        ),
                  ),
                  const SizedBox(height: 18),
                  ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 620),
                    child: const Text(
                      'Demonstração do fluxo conversacional com análise de intenção, persistência de contexto e transbordo para atendimento humano.',
                      style: TextStyle(
                        color: Colors.white70,
                        fontSize: 17,
                        height: 1.55,
                      ),
                    ),
                  ),
                  const SizedBox(height: 26),
                  Wrap(
                    spacing: 9,
                    runSpacing: 9,
                    children: const <Widget>[
                      _DarkFeature(
                        icon: Icons.hub_outlined,
                        label: 'Omnichannel',
                      ),
                      _DarkFeature(
                        icon: Icons.psychology_alt_outlined,
                        label: 'NLP / IA',
                      ),
                      _DarkFeature(
                        icon: Icons.support_agent_rounded,
                        label: 'Transbordo humano',
                      ),
                      _DarkFeature(
                        icon: Icons.sync_rounded,
                        label: 'Contexto persistente',
                      ),
                    ],
                  ),
                  const Spacer(),
                  TextButton.icon(
                    onPressed: controller.resetDemo,
                    style: TextButton.styleFrom(foregroundColor: Colors.white70),
                    icon: const Icon(Icons.restart_alt_rounded, size: 18),
                    label: const Text('Reiniciar dados da demonstração'),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _RolePanel extends StatelessWidget {
  const _RolePanel({required this.controller});

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return Container(
      color: AppColors.canvas,
      constraints: const BoxConstraints(minHeight: 520),
      child: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.symmetric(horizontal: 30, vertical: 38),
          child: Align(
            alignment: Alignment.topCenter,
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 620),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: <Widget>[
                  Text(
                    'Escolha a experiência',
                    style: Theme.of(context).textTheme.headlineMedium,
                  ),
                  const SizedBox(height: 10),
                  Text(
                    'As duas áreas compartilham o mesmo estado em memória. Assim, uma solicitação feita pelo cliente aparece imediatamente na fila administrativa.',
                    style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                          color: AppColors.muted,
                        ),
                  ),
                  const SizedBox(height: 28),
                  _RoleCard(
                    icon: Icons.phone_iphone_rounded,
                    title: 'Área do cliente',
                    subtitle:
                        'Abra um atendimento, teste internet lenta, altere o canal e solicite ajuda para uma cobrança.',
                    tags: const <String>[
                      'Chat',
                      'Continuidade',
                      'Sessões',
                    ],
                    actionLabel: 'Entrar como cliente',
                    onTap: () {
                      Navigator.of(context).push(
                        MaterialPageRoute<void>(
                          builder: (BuildContext context) =>
                              CustomerPortal(controller: controller),
                        ),
                      );
                    },
                  ),
                  const SizedBox(height: 16),
                  _RoleCard(
                    icon: Icons.space_dashboard_outlined,
                    title: 'Dashboard administrativo',
                    subtitle:
                        'Monitore a fila, assuma atendimentos, consulte o resumo da IA e responda ao cliente.',
                    tags: const <String>[
                      'Fila em tempo real',
                      'Resumo da IA',
                      'Resposta manual',
                    ],
                    actionLabel: 'Entrar como atendente',
                    emphasized: true,
                    onTap: () {
                      Navigator.of(context).push(
                        MaterialPageRoute<void>(
                          builder: (BuildContext context) =>
                              AdminPortal(controller: controller),
                        ),
                      );
                    },
                  ),
                  const SizedBox(height: 22),
                  SurfaceCard(
                    color: const Color(0xFFFFF7F7),
                    borderColor: const Color(0xFFFFD3D7),
                    padding: const EdgeInsets.all(16),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: <Widget>[
                        const Icon(
                          Icons.info_outline_rounded,
                          color: AppColors.claroRed,
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Text(
                            'Este projeto usa dados simulados e não envia informações para serviços externos. Os pontos de integração estão documentados no README.',
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
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

class _RoleCard extends StatelessWidget {
  const _RoleCard({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.tags,
    required this.actionLabel,
    required this.onTap,
    this.emphasized = false,
  });

  final IconData icon;
  final String title;
  final String subtitle;
  final List<String> tags;
  final String actionLabel;
  final VoidCallback onTap;
  final bool emphasized;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      onTap: onTap,
      borderColor: emphasized ? const Color(0xFFFFB8BE) : AppColors.border,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Container(
                width: 50,
                height: 50,
                decoration: BoxDecoration(
                  color: emphasized
                      ? const Color(0xFFFFE6E8)
                      : const Color(0xFFEEEFF2),
                  borderRadius: BorderRadius.circular(16),
                ),
                child: Icon(
                  icon,
                  color: emphasized ? AppColors.claroRed : AppColors.ink,
                ),
              ),
              const SizedBox(width: 15),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    Text(title, style: Theme.of(context).textTheme.titleLarge),
                    const SizedBox(height: 6),
                    Text(
                      subtitle,
                      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                            color: AppColors.muted,
                          ),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 17),
          Wrap(
            spacing: 7,
            runSpacing: 7,
            children: tags
                .map(
                  (String tag) => Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 9, vertical: 5),
                    decoration: BoxDecoration(
                      color: AppColors.canvas,
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: Text(
                      tag,
                      style: Theme.of(context).textTheme.bodySmall?.copyWith(
                            fontWeight: FontWeight.w700,
                          ),
                    ),
                  ),
                )
                .toList(),
          ),
          const SizedBox(height: 18),
          Row(
            children: <Widget>[
              Text(
                actionLabel,
                style: TextStyle(
                  color: emphasized ? AppColors.claroRed : AppColors.ink,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(width: 7),
              Icon(
                Icons.arrow_forward_rounded,
                size: 19,
                color: emphasized ? AppColors.claroRed : AppColors.ink,
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _DarkFeature extends StatelessWidget {
  const _DarkFeature({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.07),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.white12),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Icon(icon, color: Colors.white70, size: 17),
          const SizedBox(width: 7),
          Text(
            label,
            style: const TextStyle(
              color: Colors.white,
              fontSize: 12,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
  }
}

class _ArchitecturePainter extends CustomPainter {
  const _ArchitecturePainter();

  @override
  void paint(Canvas canvas, Size size) {
    final Paint linePaint = Paint()
      ..color = Colors.white.withValues(alpha: 0.055)
      ..strokeWidth = 1;
    final Paint nodePaint = Paint()
      ..color = AppColors.claroRed.withValues(alpha: 0.32)
      ..style = PaintingStyle.fill;

    final List<Offset> nodes = <Offset>[
      Offset(size.width * 0.12, size.height * 0.18),
      Offset(size.width * 0.42, size.height * 0.12),
      Offset(size.width * 0.78, size.height * 0.20),
      Offset(size.width * 0.25, size.height * 0.48),
      Offset(size.width * 0.62, size.height * 0.42),
      Offset(size.width * 0.88, size.height * 0.58),
      Offset(size.width * 0.18, size.height * 0.78),
      Offset(size.width * 0.54, size.height * 0.82),
    ];

    for (int index = 0; index < nodes.length - 1; index += 1) {
      canvas.drawLine(nodes[index], nodes[index + 1], linePaint);
    }
    canvas.drawLine(nodes[0], nodes[3], linePaint);
    canvas.drawLine(nodes[2], nodes[4], linePaint);
    canvas.drawLine(nodes[3], nodes[6], linePaint);
    canvas.drawLine(nodes[4], nodes[7], linePaint);

    for (final Offset node in nodes) {
      canvas.drawCircle(node, 4, nodePaint);
      canvas.drawCircle(
        node,
        11,
        Paint()
          ..color = Colors.white.withValues(alpha: 0.035)
          ..style = PaintingStyle.fill,
      );
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
