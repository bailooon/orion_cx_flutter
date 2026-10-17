import 'dart:async';

import 'package:flutter/material.dart';

import '../core/app_theme.dart';
import '../core/dashboard.dart';
import '../core/orion_controller.dart';
import '../widgets/common_widgets.dart';

/// Operational dashboard: volume, automation, AI agents, queue and security.
class OperationsDashboardPage extends StatelessWidget {
  const OperationsDashboardPage({required this.controller, super.key});

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return _DashboardFrame(
      controller: controller,
      title: 'Indicadores da operação',
      subtitle:
          'Volume, automação pela IA, agentes, fila e segurança do chatbot — atualizado a cada 5 segundos.',
      builder: (BuildContext context, DashboardData data) =>
          _OperationsBody(data: data),
    );
  }
}

/// Satisfaction dashboard built on the post-attendance survey.
class SatisfactionDashboardPage extends StatelessWidget {
  const SatisfactionDashboardPage({required this.controller, super.key});

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return _DashboardFrame(
      controller: controller,
      title: 'Satisfação do cliente',
      subtitle:
          'Pesquisa respondida ao fim de cada atendimento, comparando IA e atendimento humano.',
      builder: (BuildContext context, DashboardData data) =>
          _SatisfactionBody(data: data),
    );
  }
}

/// Shared frame: loads the dashboard on open, refreshes it on a timer while
/// visible, and handles the loading and error states.
class _DashboardFrame extends StatefulWidget {
  const _DashboardFrame({
    required this.controller,
    required this.title,
    required this.subtitle,
    required this.builder,
  });

  final OrionController controller;
  final String title;
  final String subtitle;
  final Widget Function(BuildContext context, DashboardData data) builder;

  @override
  State<_DashboardFrame> createState() => _DashboardFrameState();
}

class _DashboardFrameState extends State<_DashboardFrame> {
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      widget.controller.loadDashboard();
    });
    _timer = Timer.periodic(const Duration(seconds: 5), (_) {
      widget.controller.loadDashboard();
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (BuildContext context, Widget? child) {
        final DashboardData? data = widget.controller.dashboard;
        return ListView(
          padding: const EdgeInsets.fromLTRB(18, 18, 18, 28),
          children: <Widget>[
            Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: <Widget>[
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Text(widget.title,
                          style: Theme.of(context).textTheme.headlineMedium),
                      const SizedBox(height: 5),
                      Text(
                        widget.subtitle,
                        style: Theme.of(context)
                            .textTheme
                            .bodyMedium
                            ?.copyWith(color: AppColors.muted),
                      ),
                    ],
                  ),
                ),
                IconButton(
                  tooltip: 'Atualizar agora',
                  onPressed: widget.controller.loadDashboard,
                  icon: widget.controller.dashboardLoading
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.refresh_rounded),
                ),
              ],
            ),
            const SizedBox(height: 16),
            if (widget.controller.dashboardError != null)
              _Notice(
                icon: Icons.error_outline_rounded,
                color: AppColors.claroRed,
                text: widget.controller.dashboardError!,
              ),
            if (data == null && widget.controller.dashboardError == null)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 60),
                child: Center(child: CircularProgressIndicator()),
              )
            else if (data != null)
              widget.builder(context, data),
          ],
        );
      },
    );
  }
}

// --- Operations ------------------------------------------------------------

class _OperationsBody extends StatelessWidget {
  const _OperationsBody({required this.data});

  final DashboardData data;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        _MetricWrap(metrics: <Widget>[
          MetricCard(
            label: 'Atendimentos',
            value: '${data.totalConversations}',
            icon: Icons.forum_outlined,
            accent: AppColors.info,
            helper: '${data.resolvedConversations} concluídos',
          ),
          MetricCard(
            label: 'Automação pela IA',
            value: _pct(data.automationRate),
            icon: Icons.smart_toy_outlined,
            accent: AppColors.success,
            helper: 'Concluídos sem humano',
          ),
          MetricCard(
            label: 'Transbordo humano',
            value: _pct(data.handoffRate),
            icon: Icons.support_agent_rounded,
            accent: AppColors.warning,
            helper: '${data.waitingConversations} na fila agora',
          ),
          MetricCard(
            label: 'Tempo médio',
            value: '${data.avgHandlingMinutes.toStringAsFixed(1)} min',
            icon: Icons.timer_outlined,
            accent: AppColors.purple,
            helper: 'Da abertura à conclusão',
          ),
          MetricCard(
            label: 'Confiança média',
            value: _pct(data.avgConfidence * 100),
            icon: Icons.psychology_alt_outlined,
            accent: AppColors.info,
            helper: 'Limiar: ${_pct(data.confidenceThreshold * 100)}',
          ),
          MetricCard(
            label: 'Troca de canal',
            value: _pct(data.crossChannelRate),
            icon: Icons.sync_alt_rounded,
            accent: AppColors.purple,
            helper: 'Jornadas multicanal',
          ),
        ]),
        const SizedBox(height: 16),
        _TwoColumns(
          left: _Section(
            title: 'Intenções identificadas',
            subtitle: 'O que os clientes pedem, segundo o motor de NLU.',
            child: _BarList(
              items: data.intents
                  .map((BucketData b) =>
                      _BarItem(_intentLabel(b.key), b.count.toDouble(), '${b.count}'))
                  .toList(),
              color: AppColors.info,
            ),
          ),
          right: _Section(
            title: 'Canais',
            subtitle: 'Último canal usado em cada atendimento.',
            child: _BarList(
              items: data.channels
                  .map((BucketData b) =>
                      _BarItem(_channelLabel(b.key), b.count.toDouble(), '${b.count}'))
                  .toList(),
              color: AppColors.purple,
            ),
          ),
        ),
        const SizedBox(height: 16),
        _Section(
          title: 'Agentes de IA e fila',
          subtitle:
              'Motor: ${data.engine == 'claude-function-calling' ? 'Claude com function calling' : 'plano determinístico (sem chave de API)'}. '
              'A fila limita chamadas simultâneas e descarta o excesso em vez de travar o atendimento.',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: <Widget>[
              _TwoColumns(
                left: _QueueCard(title: 'Fila dos agentes de IA', queue: data.agentQueue),
                right: _QueueCard(title: 'Fila do NLU (classificação)', queue: data.nluQueue),
              ),
              const SizedBox(height: 14),
              _MetricWrap(metrics: <Widget>[
                MetricCard(
                  label: 'Chamadas de ferramenta',
                  value: '${data.toolCalls}',
                  icon: Icons.build_circle_outlined,
                  accent: AppColors.info,
                  helper: '${data.toolErrors} com erro · ${data.avgToolLatencyMs.toStringAsFixed(1)} ms',
                ),
                MetricCard(
                  label: 'Descartes por sobrecarga',
                  value: '${data.queueSheds}',
                  icon: Icons.speed_rounded,
                  accent: AppColors.warning,
                  helper: 'Respondidos em contingência',
                ),
              ]),
              const SizedBox(height: 14),
              _TwoColumns(
                left: _BarList(
                  items: data.toolsUsage
                      .map((BucketData b) => _BarItem(b.key, b.count.toDouble(), '${b.count}'))
                      .toList(),
                  color: AppColors.success,
                  emptyText: 'Nenhuma ferramenta chamada ainda.',
                ),
                right: _AgentList(agents: data.agents),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        _Section(
          title: 'Segurança e double check',
          subtitle:
              'Entradas bloqueadas, ações sensíveis confirmadas pelo cliente e reverificadas pelo servidor, respostas da IA corrigidas.',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: <Widget>[
              _MetricWrap(metrics: <Widget>[
                MetricCard(
                  label: 'Injeções bloqueadas',
                  value: '${data.securityBlocks}',
                  icon: Icons.gpp_bad_outlined,
                  accent: AppColors.claroRed,
                  helper: 'Máx. ${data.maxInputChars} caracteres por mensagem',
                ),
                MetricCard(
                  label: 'Limite de mensagens',
                  value: '${data.rateLimited}',
                  icon: Icons.block_rounded,
                  accent: AppColors.warning,
                  helper: '${data.rateLimitPerMinute}/min por cliente',
                ),
                MetricCard(
                  label: 'Ferramentas barradas',
                  value: '${data.toolsBlocked}',
                  icon: Icons.policy_outlined,
                  accent: AppColors.purple,
                  helper: 'Fora da lista ou argumento inválido',
                ),
                MetricCard(
                  label: 'Confirmações',
                  value: '${data.confirmationsAccepted}/${data.confirmationsRequested}',
                  icon: Icons.verified_user_outlined,
                  accent: AppColors.success,
                  helper: '${data.confirmationsDeclined} recusadas · ${data.confirmationsRejected} barradas',
                ),
                MetricCard(
                  label: 'Respostas corrigidas',
                  value: '${data.repliesCorrected}',
                  icon: Icons.fact_check_outlined,
                  accent: AppColors.info,
                  helper: 'Verificação da resposta da IA',
                ),
              ]),
              const SizedBox(height: 14),
              Text('Trilha de auditoria recente',
                  style: Theme.of(context).textTheme.titleSmall),
              const SizedBox(height: 8),
              _AuditList(entries: data.recentAudit),
            ],
          ),
        ),
        const SizedBox(height: 16),
        _Section(
          title: 'Chamados',
          subtitle: 'Protocolos por status.',
          child: _BarList(
            items: <_BarItem>[
              _BarItem('Abertos', data.ticketsOpen.toDouble(), '${data.ticketsOpen}'),
              _BarItem('Em atendimento', data.ticketsInProgress.toDouble(), '${data.ticketsInProgress}'),
              _BarItem('Concluídos', data.ticketsResolved.toDouble(), '${data.ticketsResolved}'),
            ],
            color: AppColors.warning,
          ),
        ),
      ],
    );
  }
}

class _QueueCard extends StatelessWidget {
  const _QueueCard({required this.title, required this.queue});

  final String title;
  final QueueData queue;

  @override
  Widget build(BuildContext context) {
    final TextTheme text = Theme.of(context).textTheme;
    if (!queue.available) {
      return _Notice(
        icon: Icons.info_outline_rounded,
        color: AppColors.muted,
        text: '$title: indisponível no momento.',
      );
    }
    final double utilization = queue.utilization.clamp(0, 1).toDouble();
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: AppColors.canvas,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Text(title, style: text.titleSmall),
          const SizedBox(height: 10),
          ClipRRect(
            borderRadius: BorderRadius.circular(999),
            child: LinearProgressIndicator(
              value: utilization,
              minHeight: 8,
              backgroundColor: AppColors.border,
              color: utilization > 0.8 ? AppColors.claroRed : AppColors.success,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            'Em execução: ${queue.inFlight}/${queue.capacity} · aguardando: ${queue.queued}/${queue.maxQueue}',
            style: text.bodySmall,
          ),
          Text(
            'Processados: ${queue.processed} · descartados: ${queue.rejected + queue.timedOut} · '
            'pico: ${queue.peakInFlight} · espera média: ${queue.avgWaitMs.toStringAsFixed(1)} ms',
            style: text.bodySmall?.copyWith(color: AppColors.muted),
          ),
        ],
      ),
    );
  }
}

class _AgentList extends StatelessWidget {
  const _AgentList({required this.agents});

  final List<AgentData> agents;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: agents
          .map(
            (AgentData agent) => Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: <Widget>[
                  Text(agent.name,
                      style: Theme.of(context)
                          .textTheme
                          .bodyMedium
                          ?.copyWith(fontWeight: FontWeight.w800)),
                  const SizedBox(height: 4),
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: agent.tools
                        .map((String tool) => _Tag(text: tool))
                        .toList(),
                  ),
                ],
              ),
            ),
          )
          .toList(),
    );
  }
}

class _AuditList extends StatelessWidget {
  const _AuditList({required this.entries});

  final List<AuditData> entries;

  Color _colorFor(AuditData entry) {
    switch (entry.kind) {
      case 'security_block':
      case 'confirmation_rejected':
      case 'rate_limited':
        return AppColors.claroRed;
      case 'queue_shed':
      case 'reply_corrected':
        return AppColors.warning;
      case 'confirmation_accepted':
        return AppColors.success;
      default:
        return entry.status == 'bloqueada' || entry.status == 'erro'
            ? AppColors.claroRed
            : AppColors.info;
    }
  }

  @override
  Widget build(BuildContext context) {
    if (entries.isEmpty) {
      return const _Notice(
        icon: Icons.history_rounded,
        color: AppColors.muted,
        text: 'Sem eventos de IA nos últimos 7 dias.',
      );
    }
    final TextTheme text = Theme.of(context).textTheme;
    return Column(
      children: entries.take(12).map((AuditData entry) {
        final Color color = _colorFor(entry);
        final DateTime? at = entry.createdAt?.toLocal();
        final String time = at == null
            ? ''
            : '${at.hour.toString().padLeft(2, '0')}:${at.minute.toString().padLeft(2, '0')}';
        return Container(
          margin: const EdgeInsets.only(bottom: 6),
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
          decoration: BoxDecoration(
            color: color.withValues(alpha: 0.06),
            borderRadius: BorderRadius.circular(10),
            border: Border.all(color: color.withValues(alpha: 0.25)),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Container(
                width: 8,
                height: 8,
                margin: const EdgeInsets.only(top: 5, right: 10),
                decoration: BoxDecoration(color: color, shape: BoxShape.circle),
              ),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    Text(
                      [
                        entry.kindLabel,
                        if (entry.tool.isNotEmpty) entry.tool,
                        if (entry.status.isNotEmpty) entry.status,
                      ].join(' · '),
                      style: text.bodySmall?.copyWith(fontWeight: FontWeight.w800),
                    ),
                    if (entry.agent.isNotEmpty || entry.detail.isNotEmpty)
                      Text(
                        [
                          if (entry.agent.isNotEmpty) entry.agent,
                          if (entry.detail.isNotEmpty) entry.detail,
                        ].join(' — '),
                        style: text.bodySmall?.copyWith(color: AppColors.muted),
                      ),
                  ],
                ),
              ),
              Text(time, style: text.bodySmall?.copyWith(color: AppColors.muted)),
            ],
          ),
        );
      }).toList(),
    );
  }
}

// --- Satisfaction ------------------------------------------------------------

class _SatisfactionBody extends StatelessWidget {
  const _SatisfactionBody({required this.data});

  final DashboardData data;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        _MetricWrap(metrics: <Widget>[
          MetricCard(
            label: 'CSAT',
            value: _pct(data.csat),
            icon: Icons.sentiment_satisfied_alt_rounded,
            accent: AppColors.success,
            helper: 'Notas 4 e 5',
          ),
          MetricCard(
            label: 'Nota média',
            value: '${data.csatAverage.toStringAsFixed(2)} / 5',
            icon: Icons.star_rounded,
            accent: const Color(0xFFF5A300),
            helper: '${data.surveyResponses} respostas',
          ),
          MetricCard(
            label: 'Índice tipo NPS',
            value: data.npsLike.toStringAsFixed(0),
            icon: Icons.trending_up_rounded,
            accent: AppColors.purple,
            helper: 'Notas 5 menos notas 1 a 3',
          ),
          MetricCard(
            label: 'Taxa de resposta',
            value: _pct(data.surveyResponseRate),
            icon: Icons.how_to_vote_outlined,
            accent: AppColors.info,
            helper: 'Dos atendimentos concluídos',
          ),
        ]),
        const SizedBox(height: 16),
        _TwoColumns(
          left: _Section(
            title: 'Distribuição das notas',
            subtitle: 'Quantas respostas em cada nota.',
            child: _DistributionChart(distribution: data.distribution),
          ),
          right: _Section(
            title: 'IA × atendimento humano',
            subtitle: 'Satisfação por quem resolveu o caso.',
            child: _SegmentList(
              segments: data.byResolver,
              labelFor: (String key) =>
                  key == 'ia' ? 'Assistente Orion (IA)' : 'Atendente humano',
            ),
          ),
        ),
        const SizedBox(height: 16),
        _Section(
          title: 'Últimos 7 dias',
          subtitle: 'Nota média das respostas e atendimentos concluídos por dia.',
          child: _DailyChart(days: data.daily),
        ),
        const SizedBox(height: 16),
        _TwoColumns(
          left: _Section(
            title: 'Por intenção',
            subtitle: 'Onde a experiência é melhor ou pior.',
            child: _SegmentList(segments: data.byIntent, labelFor: _intentLabel),
          ),
          right: _Section(
            title: 'Por atendente',
            subtitle: 'Inclui o assistente de IA.',
            child: _SegmentList(segments: data.byAgent, labelFor: (String key) => key),
          ),
        ),
        const SizedBox(height: 16),
        _Section(
          title: 'Comentários recentes',
          subtitle: 'Só o primeiro nome do cliente é exibido (LGPD).',
          child: _SurveyList(surveys: data.recentSurveys),
        ),
      ],
    );
  }
}

class _DistributionChart extends StatelessWidget {
  const _DistributionChart({required this.distribution});

  final List<int> distribution;

  static const List<Color> _colors = <Color>[
    Color(0xFFD64545),
    Color(0xFFE8833A),
    Color(0xFFE0B526),
    Color(0xFF7BBF5A),
    Color(0xFF2E9E62),
  ];

  @override
  Widget build(BuildContext context) {
    final int top = distribution.fold<int>(0, (int a, int b) => a > b ? a : b);
    return SizedBox(
      height: 170,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: List<Widget>.generate(5, (int index) {
          final int count = distribution[index];
          final double factor = top == 0 ? 0 : count / top;
          return Expanded(
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 6),
              child: Column(
                mainAxisAlignment: MainAxisAlignment.end,
                children: <Widget>[
                  Text('$count', style: Theme.of(context).textTheme.bodySmall),
                  const SizedBox(height: 4),
                  Container(
                    height: 110 * factor + 2,
                    decoration: BoxDecoration(
                      color: _colors[index],
                      borderRadius: BorderRadius.circular(6),
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text('${index + 1}★',
                      style: Theme.of(context)
                          .textTheme
                          .bodySmall
                          ?.copyWith(fontWeight: FontWeight.w800)),
                ],
              ),
            ),
          );
        }),
      ),
    );
  }
}

class _DailyChart extends StatelessWidget {
  const _DailyChart({required this.days});

  final List<DayData> days;

  @override
  Widget build(BuildContext context) {
    if (days.isEmpty) {
      return const _Notice(
        icon: Icons.insights_rounded,
        color: AppColors.muted,
        text: 'Sem dados no período.',
      );
    }
    final TextTheme text = Theme.of(context).textTheme;
    return SizedBox(
      height: 170,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: days.map((DayData day) {
          final double factor = day.average / 5;
          return Expanded(
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 4),
              child: Column(
                mainAxisAlignment: MainAxisAlignment.end,
                children: <Widget>[
                  Text(
                    day.responses == 0 ? '—' : day.average.toStringAsFixed(1),
                    style: text.bodySmall?.copyWith(fontWeight: FontWeight.w700),
                  ),
                  const SizedBox(height: 4),
                  Container(
                    height: 100 * factor + 2,
                    decoration: BoxDecoration(
                      color: AppColors.success.withValues(
                          alpha: day.responses == 0 ? 0.15 : 0.85),
                      borderRadius: BorderRadius.circular(6),
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(day.shortLabel, style: text.bodySmall),
                  Text('${day.resolved} concl.',
                      style: text.bodySmall?.copyWith(color: AppColors.muted, fontSize: 10)),
                ],
              ),
            ),
          );
        }).toList(),
      ),
    );
  }
}

class _SegmentList extends StatelessWidget {
  const _SegmentList({required this.segments, required this.labelFor});

  final List<SegmentData> segments;
  final String Function(String key) labelFor;

  @override
  Widget build(BuildContext context) {
    if (segments.isEmpty) {
      return const _Notice(
        icon: Icons.inbox_outlined,
        color: AppColors.muted,
        text: 'Nenhuma resposta ainda.',
      );
    }
    return _BarList(
      items: segments
          .map((SegmentData s) => _BarItem(
                '${labelFor(s.key)} (${s.responses})',
                s.average,
                '${s.average.toStringAsFixed(1)} · CSAT ${_pct(s.csat)}',
              ))
          .toList(),
      color: AppColors.success,
      maxValue: 5,
    );
  }
}

class _SurveyList extends StatelessWidget {
  const _SurveyList({required this.surveys});

  final List<SurveyData> surveys;

  @override
  Widget build(BuildContext context) {
    if (surveys.isEmpty) {
      return const _Notice(
        icon: Icons.chat_bubble_outline_rounded,
        color: AppColors.muted,
        text: 'Nenhuma pesquisa respondida ainda.',
      );
    }
    final TextTheme text = Theme.of(context).textTheme;
    return Column(
      children: surveys.map((SurveyData survey) {
        return Container(
          margin: const EdgeInsets.only(bottom: 8),
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: AppColors.canvas,
            borderRadius: BorderRadius.circular(12),
            border: Border.all(color: AppColors.border),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text('${survey.score}★',
                  style: text.titleMedium?.copyWith(color: const Color(0xFFF5A300))),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    Text(
                      survey.comment.isEmpty ? 'Sem comentário.' : '"${survey.comment}"',
                      style: text.bodyMedium?.copyWith(
                        fontStyle: survey.comment.isEmpty ? FontStyle.italic : FontStyle.normal,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      '${survey.customer} · ${survey.conversationId} · ${_intentLabel(survey.intent)} · '
                      '${survey.resolvedBy == 'ia' ? 'resolvido pela IA' : 'resolvido por ${survey.agent}'}',
                      style: text.bodySmall?.copyWith(color: AppColors.muted),
                    ),
                  ],
                ),
              ),
            ],
          ),
        );
      }).toList(),
    );
  }
}

// --- Building blocks ---------------------------------------------------------

class _BarItem {
  const _BarItem(this.label, this.value, this.display);

  final String label;
  final double value;
  final String display;
}

class _BarList extends StatelessWidget {
  const _BarList({
    required this.items,
    required this.color,
    this.maxValue,
    this.emptyText = 'Sem dados ainda.',
  });

  final List<_BarItem> items;
  final Color color;
  final double? maxValue;
  final String emptyText;

  @override
  Widget build(BuildContext context) {
    if (items.isEmpty) {
      return _Notice(icon: Icons.inbox_outlined, color: AppColors.muted, text: emptyText);
    }
    final double top = maxValue ??
        items.fold<double>(0, (double a, _BarItem b) => a > b.value ? a : b.value);
    final TextTheme text = Theme.of(context).textTheme;
    return Column(
      children: items.map((_BarItem item) {
        final double factor = top <= 0 ? 0 : (item.value / top).clamp(0, 1).toDouble();
        return Padding(
          padding: const EdgeInsets.only(bottom: 10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Row(
                children: <Widget>[
                  Expanded(
                    child: Text(item.label,
                        style: text.bodySmall, overflow: TextOverflow.ellipsis),
                  ),
                  Text(item.display,
                      style: text.bodySmall?.copyWith(fontWeight: FontWeight.w800)),
                ],
              ),
              const SizedBox(height: 4),
              ClipRRect(
                borderRadius: BorderRadius.circular(999),
                child: LinearProgressIndicator(
                  value: factor,
                  minHeight: 8,
                  backgroundColor: AppColors.border,
                  color: color,
                ),
              ),
            ],
          ),
        );
      }).toList(),
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.title, required this.subtitle, required this.child});

  final String title;
  final String subtitle;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: <Widget>[
          Text(title, style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 3),
          Text(
            subtitle,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(color: AppColors.muted),
          ),
          const SizedBox(height: 14),
          child,
        ],
      ),
    );
  }
}

class _MetricWrap extends StatelessWidget {
  const _MetricWrap({required this.metrics});

  final List<Widget> metrics;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (BuildContext context, BoxConstraints constraints) {
        final int columns = constraints.maxWidth >= 1100
            ? 3
            : constraints.maxWidth >= 620
                ? 2
                : 1;
        const double gap = 10;
        final double width = (constraints.maxWidth - gap * (columns - 1)) / columns;
        return Wrap(
          spacing: gap,
          runSpacing: gap,
          children: metrics
              .map((Widget metric) => SizedBox(width: width, child: metric))
              .toList(),
        );
      },
    );
  }
}

class _TwoColumns extends StatelessWidget {
  const _TwoColumns({required this.left, required this.right});

  final Widget left;
  final Widget right;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (BuildContext context, BoxConstraints constraints) {
        if (constraints.maxWidth < 860) {
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: <Widget>[left, const SizedBox(height: 14), right],
          );
        }
        return Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Expanded(child: left),
            const SizedBox(width: 14),
            Expanded(child: right),
          ],
        );
      },
    );
  }
}

class _Notice extends StatelessWidget {
  const _Notice({required this.icon, required this.color, required this.text});

  final IconData icon;
  final Color color;
  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        children: <Widget>[
          Icon(icon, size: 18, color: color),
          const SizedBox(width: 8),
          Expanded(
            child: Text(text,
                style: Theme.of(context).textTheme.bodySmall?.copyWith(color: color)),
          ),
        ],
      ),
    );
  }
}

class _Tag extends StatelessWidget {
  const _Tag({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: AppColors.canvas,
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: AppColors.border),
      ),
      child: Text(text, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700)),
    );
  }
}

String _pct(double value) => '${value.toStringAsFixed(value >= 10 ? 0 : 1)}%';

String _intentLabel(String key) {
  switch (key) {
    case 'SUPORTE_TECNICO':
      return 'Suporte técnico';
    case 'CONTESTACAO_FATURA':
      return 'Contestação de fatura';
    case 'SEGUNDA_VIA_FATURA':
      return 'Segunda via';
    case 'UPGRADE_PLANO':
      return 'Upgrade de plano';
    case 'CANCELAMENTO':
      return 'Cancelamento';
    case 'CONSULTA_CONSUMO':
      return 'Consumo';
    case 'STATUS_CHAMADO':
      return 'Status de chamado';
    case 'SAUDACAO':
      return 'Saudação';
    case 'EM_ANALISE':
      return 'Em análise';
    default:
      return key;
  }
}

String _channelLabel(String key) {
  switch (key) {
    case 'appClaro':
      return 'App Claro';
    case 'webPortal':
      return 'Web Portal';
    case 'whatsapp':
      return 'WhatsApp';
    default:
      return key;
  }
}
