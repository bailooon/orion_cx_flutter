import 'package:flutter/material.dart';

import '../core/app_theme.dart';
import '../core/models.dart';
import '../core/orion_controller.dart';
import '../widgets/common_widgets.dart';

/// Customer-facing follow-up screen.
///
/// Brings together the three things a customer needs after talking to Orion:
/// the protocols they can track (RF006), the notifications the platform pushed
/// to them (RF009), and the offers derived from their own history (RF007).
class TicketsScreen extends StatefulWidget {
  const TicketsScreen({required this.controller, super.key});

  final OrionController controller;

  @override
  State<TicketsScreen> createState() => _TicketsScreenState();
}

class _TicketsScreenState extends State<TicketsScreen> {
  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_onChanged);
    // The socket keeps this screen live, but a REST refresh guarantees fresh
    // recommendations when the screen is opened directly.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      widget.controller.refresh();
    });
  }

  void _onChanged() {
    if (mounted) {
      setState(() {});
    }
  }

  @override
  void dispose() {
    widget.controller.removeListener(_onChanged);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final OrionController controller = widget.controller;
    final List<Ticket> tickets = controller.tickets;
    final List<AppNotification> notifications = controller.notifications;
    final List<Recommendation> recommendations = controller.recommendations;

    return Scaffold(
      backgroundColor: AppColors.canvas,
      appBar: AppBar(
        title: const Text('Meus chamados'),
        actions: <Widget>[
          if (controller.unreadNotifications > 0)
            TextButton.icon(
              onPressed: controller.markAllNotificationsRead,
              icon: const Icon(Icons.done_all_rounded, size: 18),
              label: const Text('Marcar tudo como lido'),
            ),
          IconButton(
            tooltip: 'Atualizar',
            onPressed: controller.refresh,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SingleChildScrollView(
        child: PageContainer(
          maxWidth: 1080,
          child: LayoutBuilder(
            builder: (BuildContext context, BoxConstraints constraints) {
              final bool wide = constraints.maxWidth >= 840;
              final Widget left = Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  _SectionTitle(
                    title: 'Protocolos',
                    subtitle:
                        'Cada chamado nasce de uma conversa e acompanha o status dela em tempo real.',
                  ),
                  const SizedBox(height: 12),
                  if (tickets.isEmpty)
                    const _EmptyState(
                      icon: Icons.inbox_outlined,
                      message:
                          'Você ainda não tem chamados. Fale com o Orion pelo chat para abrir um.',
                    )
                  else
                    ...tickets.map((Ticket ticket) => Padding(
                          padding: const EdgeInsets.only(bottom: 12),
                          child: _TicketCard(ticket: ticket),
                        )),
                ],
              );

              final Widget right = Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  _SectionTitle(
                    title: 'Notificações',
                    subtitle:
                        'Avisos enviados pelo serviço de notificação a cada mudança de status.',
                  ),
                  const SizedBox(height: 12),
                  if (notifications.isEmpty)
                    const _EmptyState(
                      icon: Icons.notifications_none_rounded,
                      message: 'Nenhuma notificação por enquanto.',
                    )
                  else
                    ...notifications.take(8).map(
                          (AppNotification item) => Padding(
                            padding: const EdgeInsets.only(bottom: 10),
                            child: _NotificationCard(
                              notification: item,
                              onRead: () =>
                                  widget.controller.markNotificationRead(item.id),
                            ),
                          ),
                        ),
                  const SizedBox(height: 22),
                  _SectionTitle(
                    title: 'Para você',
                    subtitle:
                        'Sugestões calculadas a partir do seu próprio histórico.',
                  ),
                  const SizedBox(height: 12),
                  ...recommendations.map(
                    (Recommendation item) => Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: _RecommendationCard(recommendation: item),
                    ),
                  ),
                ],
              );

              if (!wide) {
                return Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: <Widget>[left, const SizedBox(height: 26), right],
                );
              }
              return Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: <Widget>[
                  Expanded(flex: 6, child: left),
                  const SizedBox(width: 22),
                  Expanded(flex: 5, child: right),
                ],
              );
            },
          ),
        ),
      ),
    );
  }
}

class _SectionTitle extends StatelessWidget {
  const _SectionTitle({required this.title, required this.subtitle});

  final String title;
  final String subtitle;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: <Widget>[
        Text(title, style: Theme.of(context).textTheme.titleLarge),
        const SizedBox(height: 4),
        Text(
          subtitle,
          style: Theme.of(context)
              .textTheme
              .bodySmall
              ?.copyWith(color: AppColors.muted),
        ),
      ],
    );
  }
}

class _TicketCard extends StatelessWidget {
  const _TicketCard({required this.ticket});

  final Ticket ticket;

  Color get _statusColor {
    switch (ticket.status) {
      case TicketStatus.open:
        return AppColors.info;
      case TicketStatus.inProgress:
        return AppColors.warning;
      case TicketStatus.resolved:
        return AppColors.success;
    }
  }

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    Text(
                      ticket.title,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: 3),
                    Text(
                      '${ticket.id} • aberto em ${ticket.channel.label}',
                      style: Theme.of(context)
                          .textTheme
                          .bodySmall
                          ?.copyWith(color: AppColors.muted),
                    ),
                  ],
                ),
              ),
              Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
                decoration: BoxDecoration(
                  color: _statusColor.withValues(alpha: 0.12),
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  ticket.status.label,
                  style: TextStyle(
                    color: _statusColor,
                    fontSize: 12,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ),
            ],
          ),
          if (ticket.timeline.isNotEmpty) ...<Widget>[
            const SizedBox(height: 14),
            const Divider(height: 1),
            const SizedBox(height: 12),
            ...ticket.timeline.map(
              (TicketEvent event) => Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    Padding(
                      padding: const EdgeInsets.only(top: 5, right: 10),
                      child: Container(
                        width: 7,
                        height: 7,
                        decoration: const BoxDecoration(
                          color: AppColors.border,
                          shape: BoxShape.circle,
                        ),
                      ),
                    ),
                    Expanded(
                      child: Text(
                        event.description,
                        style: Theme.of(context).textTheme.bodySmall,
                      ),
                    ),
                    Text(
                      _formatTime(event.at),
                      style: Theme.of(context)
                          .textTheme
                          .bodySmall
                          ?.copyWith(color: AppColors.muted),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _NotificationCard extends StatelessWidget {
  const _NotificationCard({required this.notification, required this.onRead});

  final AppNotification notification;
  final VoidCallback onRead;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.all(14),
      color: notification.read ? AppColors.surface : const Color(0xFFFFF7F7),
      borderColor:
          notification.read ? AppColors.border : const Color(0xFFFFD3D7),
      onTap: notification.read ? null : onRead,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Icon(
            notification.read
                ? Icons.notifications_none_rounded
                : Icons.notifications_active_rounded,
            size: 20,
            color: notification.read ? AppColors.muted : AppColors.claroRed,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  notification.title,
                  style: Theme.of(context).textTheme.titleSmall?.copyWith(
                        fontWeight: notification.read
                            ? FontWeight.w600
                            : FontWeight.w800,
                      ),
                ),
                const SizedBox(height: 3),
                Text(
                  notification.body,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
                const SizedBox(height: 5),
                Text(
                  '${notification.channel.label} • ${_formatTime(notification.createdAt)}',
                  style: Theme.of(context)
                      .textTheme
                      .bodySmall
                      ?.copyWith(color: AppColors.muted, fontSize: 11),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _RecommendationCard extends StatelessWidget {
  const _RecommendationCard({required this.recommendation});

  final Recommendation recommendation;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Row(
            children: <Widget>[
              const Icon(Icons.auto_awesome_rounded,
                  size: 18, color: AppColors.purple),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  recommendation.title,
                  style: Theme.of(context).textTheme.titleSmall,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            recommendation.body,
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 8),
          // Showing why an offer appeared keeps the recommendation auditable
          // instead of looking like an untargeted advertisement.
          Text(
            'Por que você está vendo isso: ${recommendation.reason}',
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: AppColors.muted,
                  fontStyle: FontStyle.italic,
                ),
          ),
        ],
      ),
    );
  }
}

class _EmptyState extends StatelessWidget {
  const _EmptyState({required this.icon, required this.message});

  final IconData icon;
  final String message;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.all(22),
      child: Column(
        children: <Widget>[
          Icon(icon, size: 30, color: AppColors.muted),
          const SizedBox(height: 10),
          Text(
            message,
            textAlign: TextAlign.center,
            style: Theme.of(context)
                .textTheme
                .bodyMedium
                ?.copyWith(color: AppColors.muted),
          ),
        ],
      ),
    );
  }
}

String _formatTime(DateTime value) {
  final DateTime local = value.toLocal();
  final String hour = local.hour.toString().padLeft(2, '0');
  final String minute = local.minute.toString().padLeft(2, '0');
  final String day = local.day.toString().padLeft(2, '0');
  final String month = local.month.toString().padLeft(2, '0');
  return '$day/$month $hour:$minute';
}
