import 'package:flutter/material.dart';

import '../core/app_theme.dart';
import '../core/models.dart';

class PageContainer extends StatelessWidget {
  const PageContainer({
    required this.child,
    this.maxWidth = 1180,
    this.padding = const EdgeInsets.fromLTRB(20, 20, 20, 28),
    super.key,
  });

  final Widget child;
  final double maxWidth;
  final EdgeInsets padding;

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: Alignment.topCenter,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxWidth),
        child: Padding(
          padding: padding,
          child: child,
        ),
      ),
    );
  }
}

class BrandMark extends StatelessWidget {
  const BrandMark({
    this.compact = false,
    this.onDark = false,
    super.key,
  });

  final bool compact;
  final bool onDark;

  @override
  Widget build(BuildContext context) {
    final Color foreground = onDark ? Colors.white : AppColors.ink;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: <Widget>[
        Container(
          width: compact ? 34 : 42,
          height: compact ? 34 : 42,
          decoration: const BoxDecoration(
            color: AppColors.claroRed,
            shape: BoxShape.circle,
          ),
          child: Icon(
            Icons.auto_awesome_rounded,
            size: compact ? 18 : 22,
            color: Colors.white,
          ),
        ),
        const SizedBox(width: 10),
        Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Text(
              'ORION CX',
              style: TextStyle(
                color: foreground,
                fontSize: compact ? 15 : 17,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.8,
              ),
            ),
            if (!compact)
              Text(
                'Atendimento omnichannel',
                style: TextStyle(
                  color: onDark ? Colors.white70 : AppColors.muted,
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                ),
              ),
          ],
        ),
      ],
    );
  }
}

class SurfaceCard extends StatelessWidget {
  const SurfaceCard({
    required this.child,
    this.padding = const EdgeInsets.all(20),
    this.onTap,
    this.color = AppColors.surface,
    this.borderColor = AppColors.border,
    this.radius = 20,
    super.key,
  });

  final Widget child;
  final EdgeInsets padding;
  final VoidCallback? onTap;
  final Color color;
  final Color borderColor;
  final double radius;

  @override
  Widget build(BuildContext context) {
    final Widget content = Container(
      padding: padding,
      decoration: BoxDecoration(
        color: color,
        borderRadius: BorderRadius.circular(radius),
        border: Border.all(color: borderColor),
        boxShadow: const <BoxShadow>[
          BoxShadow(
            color: Color(0x0A000000),
            blurRadius: 18,
            offset: Offset(0, 7),
          ),
        ],
      ),
      child: child,
    );

    if (onTap == null) {
      return content;
    }

    return Semantics(
      button: true,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(radius),
        child: content,
      ),
    );
  }
}

class StatusPill extends StatelessWidget {
  const StatusPill({
    required this.status,
    this.compact = false,
    super.key,
  });

  final SupportCaseStatus status;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final Color color;
    final IconData icon;

    switch (status) {
      case SupportCaseStatus.bot:
        color = AppColors.info;
        icon = Icons.smart_toy_outlined;
        break;
      case SupportCaseStatus.waitingHuman:
        color = AppColors.warning;
        icon = Icons.schedule_rounded;
        break;
      case SupportCaseStatus.inProgress:
        color = AppColors.purple;
        icon = Icons.support_agent_rounded;
        break;
      case SupportCaseStatus.resolved:
        color = AppColors.success;
        icon = Icons.check_circle_outline_rounded;
        break;
    }

    return Container(
      padding: EdgeInsets.symmetric(
        horizontal: compact ? 9 : 11,
        vertical: compact ? 5 : 7,
      ),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Icon(icon, size: compact ? 14 : 16, color: color),
          const SizedBox(width: 6),
          Text(
            status.label,
            style: TextStyle(
              color: color,
              fontSize: compact ? 11 : 12,
              fontWeight: FontWeight.w800,
            ),
          ),
        ],
      ),
    );
  }
}

class MetricCard extends StatelessWidget {
  const MetricCard({
    required this.label,
    required this.value,
    required this.icon,
    required this.accent,
    this.helper,
    super.key,
  });

  final String label;
  final String value;
  final IconData icon;
  final Color accent;
  final String? helper;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.all(16),
      child: Row(
        children: <Widget>[
          Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              color: accent.withValues(alpha: 0.10),
              borderRadius: BorderRadius.circular(14),
            ),
            child: Icon(icon, color: accent),
          ),
          const SizedBox(width: 13),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  value,
                  style: Theme.of(context)
                      .textTheme
                      .titleLarge
                      ?.copyWith(fontSize: 23),
                ),
                Text(label, style: Theme.of(context).textTheme.bodySmall),
                if (helper != null) ...<Widget>[
                  const SizedBox(height: 2),
                  Text(
                    helper!,
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          color: accent,
                          fontWeight: FontWeight.w700,
                        ),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class ChatBubble extends StatelessWidget {
  const ChatBubble({
    required this.message,
    this.showChannel = true,
    super.key,
  });

  final ChatMessage message;
  final bool showChannel;

  @override
  Widget build(BuildContext context) {
    if (message.actor == ConversationActor.system) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: <Widget>[
            Flexible(
              child: Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 13, vertical: 8),
                decoration: BoxDecoration(
                  color: AppColors.canvas,
                  borderRadius: BorderRadius.circular(999),
                  border: Border.all(color: AppColors.border),
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    const Icon(
                      Icons.sync_alt_rounded,
                      size: 15,
                      color: AppColors.muted,
                    ),
                    const SizedBox(width: 7),
                    Flexible(
                      child: Text(
                        message.text,
                        textAlign: TextAlign.center,
                        style: Theme.of(context).textTheme.bodySmall,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      );
    }

    final bool isCustomer = message.actor == ConversationActor.customer;
    final bool isAgent = message.actor == ConversationActor.agent;
    final Color background = isCustomer
        ? AppColors.claroRed
        : isAgent
            ? const Color(0xFFF0EBFF)
            : AppColors.surface;
    final Color foreground = isCustomer ? Colors.white : AppColors.ink;
    final Color border = isCustomer
        ? AppColors.claroRed
        : isAgent
            ? const Color(0xFFD8CCFF)
            : AppColors.border;
    final String sender = isCustomer
        ? 'Você'
        : isAgent
            ? 'Atendente'
            : 'Orion IA';

    return Align(
      alignment: isCustomer ? Alignment.centerRight : Alignment.centerLeft,
      child: FractionallySizedBox(
        widthFactor: 0.84,
        alignment:
            isCustomer ? Alignment.centerRight : Alignment.centerLeft,
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 5),
          child: Container(
            padding: const EdgeInsets.fromLTRB(14, 11, 14, 10),
            decoration: BoxDecoration(
              color: background,
              borderRadius: BorderRadius.only(
                topLeft: const Radius.circular(18),
                topRight: const Radius.circular(18),
                bottomLeft: Radius.circular(isCustomer ? 18 : 4),
                bottomRight: Radius.circular(isCustomer ? 4 : 18),
              ),
              border: Border.all(color: border),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  sender,
                  style: TextStyle(
                    color: isCustomer ? Colors.white70 : AppColors.muted,
                    fontSize: 11,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  message.text,
                  style: TextStyle(
                    color: foreground,
                    fontSize: 14,
                    height: 1.4,
                  ),
                ),
                const SizedBox(height: 6),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    if (showChannel) ...<Widget>[
                      Text(
                        message.channel.shortLabel,
                        style: TextStyle(
                          color: isCustomer ? Colors.white70 : AppColors.muted,
                          fontSize: 10,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(width: 7),
                      Container(
                        width: 3,
                        height: 3,
                        decoration: BoxDecoration(
                          color:
                              isCustomer ? Colors.white54 : AppColors.muted,
                          shape: BoxShape.circle,
                        ),
                      ),
                      const SizedBox(width: 7),
                    ],
                    Text(
                      _formatTime(message.sentAt),
                      style: TextStyle(
                        color: isCustomer ? Colors.white70 : AppColors.muted,
                        fontSize: 10,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  String _formatTime(DateTime value) {
    final String hour = value.hour.toString().padLeft(2, '0');
    final String minute = value.minute.toString().padLeft(2, '0');
    return '$hour:$minute';
  }
}

class ConfidenceBar extends StatelessWidget {
  const ConfidenceBar({
    required this.value,
    super.key,
  });

  final double value;

  @override
  Widget build(BuildContext context) {
    final Color color = value >= 0.80
        ? AppColors.success
        : value >= 0.60
            ? AppColors.warning
            : AppColors.claroRed;
    final int percentage = (value * 100).round();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: <Widget>[
        Row(
          children: <Widget>[
            Expanded(
              child: Text(
                'Confiança da intenção',
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ),
            Text(
              '$percentage%',
              style: TextStyle(
                color: color,
                fontWeight: FontWeight.w900,
                fontSize: 12,
              ),
            ),
          ],
        ),
        const SizedBox(height: 7),
        ClipRRect(
          borderRadius: BorderRadius.circular(999),
          child: LinearProgressIndicator(
            value: value.clamp(0.0, 1.0).toDouble(),
            minHeight: 7,
            backgroundColor: AppColors.border,
            valueColor: AlwaysStoppedAnimation<Color>(color),
          ),
        ),
      ],
    );
  }
}
