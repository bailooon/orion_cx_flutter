import 'package:flutter/material.dart';

import '../core/app_theme.dart';
import '../core/models.dart';
import '../core/orion_controller.dart';
import '../widgets/common_widgets.dart';

class AdminPortal extends StatefulWidget {
  const AdminPortal({
    required this.controller,
    super.key,
  });

  final OrionController controller;

  @override
  State<AdminPortal> createState() => _AdminPortalState();
}

class _AdminPortalState extends State<AdminPortal> {
  int _destination = 0;
  String? _selectedCaseId;

  @override
  void initState() {
    super.initState();
    final List<SupportCase> initial = widget.controller.waitingCases;
    if (initial.isNotEmpty) {
      _selectedCaseId = initial.first.id;
    } else if (widget.controller.activeCases.isNotEmpty) {
      _selectedCaseId = widget.controller.activeCases.first.id;
    }
  }

  void _selectCase(BuildContext context, String caseId) {
    setState(() => _selectedCaseId = caseId);
    widget.controller.markCaseRead(caseId);

    if (MediaQuery.of(context).size.width < 980) {
      Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (BuildContext context) => AgentWorkspaceScreen(
            controller: widget.controller,
            caseId: caseId,
          ),
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const BrandMark(compact: true),
        actions: <Widget>[
          AnimatedBuilder(
            animation: widget.controller,
            builder: (BuildContext context, Widget? child) {
              return Padding(
                padding: const EdgeInsets.symmetric(horizontal: 2),
                child: Stack(
                  clipBehavior: Clip.none,
                  children: <Widget>[
                    IconButton(
                      tooltip: 'Eventos pendentes',
                      onPressed: () => setState(() => _destination = 0),
                      icon: const Icon(Icons.notifications_none_rounded),
                    ),
                    if (widget.controller.unattendedEvents > 0)
                      Positioned(
                        right: 7,
                        top: 7,
                        child: Container(
                          width: 17,
                          height: 17,
                          alignment: Alignment.center,
                          decoration: const BoxDecoration(
                            color: AppColors.claroRed,
                            shape: BoxShape.circle,
                          ),
                          child: Text(
                            '${widget.controller.unattendedEvents}',
                            style: const TextStyle(
                              color: Colors.white,
                              fontSize: 9,
                              fontWeight: FontWeight.w900,
                            ),
                          ),
                        ),
                      ),
                  ],
                ),
              );
            },
          ),
          IconButton(
            tooltip: 'Reiniciar demonstração',
            onPressed: widget.controller.resetDemo,
            icon: const Icon(Icons.restart_alt_rounded),
          ),
          IconButton(
            tooltip: 'Sair do dashboard',
            onPressed: () => Navigator.of(context).pop(),
            icon: const Icon(Icons.logout_rounded),
          ),
          const SizedBox(width: 8),
        ],
      ),
      body: LayoutBuilder(
        builder: (BuildContext context, BoxConstraints constraints) {
          final bool wideNavigation = constraints.maxWidth >= 760;
          final Widget content = _buildDestination();

          if (!wideNavigation) {
            return content;
          }

          return Row(
            children: <Widget>[
              NavigationRail(
                selectedIndex: _destination,
                onDestinationSelected: (int value) {
                  setState(() => _destination = value);
                },
                labelType: constraints.maxWidth >= 1120
                    ? NavigationRailLabelType.all
                    : NavigationRailLabelType.selected,
                leading: Padding(
                  padding: const EdgeInsets.only(top: 10, bottom: 18),
                  child: Container(
                    width: 40,
                    height: 40,
                    alignment: Alignment.center,
                    decoration: const BoxDecoration(
                      color: Color(0xFFFFE3E6),
                      shape: BoxShape.circle,
                    ),
                    child: const Icon(
                      Icons.support_agent_rounded,
                      color: AppColors.claroRed,
                    ),
                  ),
                ),
                destinations: const <NavigationRailDestination>[
                  NavigationRailDestination(
                    icon: Icon(Icons.inbox_outlined),
                    selectedIcon: Icon(Icons.inbox_rounded),
                    label: Text('Fila'),
                  ),
                  NavigationRailDestination(
                    icon: Icon(Icons.forum_outlined),
                    selectedIcon: Icon(Icons.forum_rounded),
                    label: Text('Em atendimento'),
                  ),
                  NavigationRailDestination(
                    icon: Icon(Icons.monitor_heart_outlined),
                    selectedIcon: Icon(Icons.monitor_heart_rounded),
                    label: Text('Monitoramento'),
                  ),
                ],
              ),
              const VerticalDivider(width: 1),
              Expanded(child: content),
            ],
          );
        },
      ),
      bottomNavigationBar: LayoutBuilder(
        builder: (BuildContext context, BoxConstraints constraints) {
          if (MediaQuery.of(context).size.width >= 760) {
            return const SizedBox.shrink();
          }
          return NavigationBar(
            selectedIndex: _destination,
            onDestinationSelected: (int value) {
              setState(() => _destination = value);
            },
            destinations: const <NavigationDestination>[
              NavigationDestination(
                icon: Icon(Icons.inbox_outlined),
                selectedIcon: Icon(Icons.inbox_rounded),
                label: 'Fila',
              ),
              NavigationDestination(
                icon: Icon(Icons.forum_outlined),
                selectedIcon: Icon(Icons.forum_rounded),
                label: 'Atendimentos',
              ),
              NavigationDestination(
                icon: Icon(Icons.monitor_heart_outlined),
                selectedIcon: Icon(Icons.monitor_heart_rounded),
                label: 'Monitor',
              ),
            ],
          );
        },
      ),
    );
  }

  Widget _buildDestination() {
    switch (_destination) {
      case 0:
        return _AdminCasesPage(
          controller: widget.controller,
          mode: _CasesMode.waiting,
          selectedCaseId: _selectedCaseId,
          onSelectCase: _selectCase,
        );
      case 1:
        return _AdminCasesPage(
          controller: widget.controller,
          mode: _CasesMode.active,
          selectedCaseId: _selectedCaseId,
          onSelectCase: _selectCase,
        );
      case 2:
        return _MonitoringPage(controller: widget.controller);
      default:
        return const SizedBox.shrink();
    }
  }
}

enum _CasesMode { waiting, active }

class _AdminCasesPage extends StatelessWidget {
  const _AdminCasesPage({
    required this.controller,
    required this.mode,
    required this.selectedCaseId,
    required this.onSelectCase,
  });

  final OrionController controller;
  final _CasesMode mode;
  final String? selectedCaseId;
  final void Function(BuildContext context, String caseId) onSelectCase;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (BuildContext context, Widget? child) {
        final List<SupportCase> items = mode == _CasesMode.waiting
            ? controller.waitingCases
            : controller.activeCases;
        final String? effectiveSelected = _effectiveSelected(items);

        return LayoutBuilder(
          builder: (BuildContext context, BoxConstraints constraints) {
            final bool splitView = constraints.maxWidth >= 980;

            if (!splitView) {
              return ListView(
                padding: const EdgeInsets.fromLTRB(16, 18, 16, 28),
                children: <Widget>[
                  _CasesHeader(mode: mode),
                  const SizedBox(height: 16),
                  _MetricsGrid(controller: controller),
                  if (mode == _CasesMode.waiting &&
                      controller.liveAlertVisible &&
                      controller.unattendedEvents > 0) ...<Widget>[
                    const SizedBox(height: 14),
                    _LiveEventBanner(controller: controller),
                  ],
                  const SizedBox(height: 18),
                  _CasesListContent(
                    controller: controller,
                    items: items,
                    selectedCaseId: effectiveSelected,
                    onSelectCase: onSelectCase,
                    compact: true,
                  ),
                ],
              );
            }

            return Padding(
              padding: const EdgeInsets.fromLTRB(18, 18, 18, 18),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  _CasesHeader(mode: mode),
                  const SizedBox(height: 14),
                  _MetricsGrid(controller: controller),
                  if (mode == _CasesMode.waiting &&
                      controller.liveAlertVisible &&
                      controller.unattendedEvents > 0) ...<Widget>[
                    const SizedBox(height: 12),
                    _LiveEventBanner(controller: controller),
                  ],
                  const SizedBox(height: 14),
                  Expanded(
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: <Widget>[
                        SizedBox(
                          width: 390,
                          child: SurfaceCard(
                            padding: EdgeInsets.zero,
                            child: _CasesListContent(
                              controller: controller,
                              items: items,
                              selectedCaseId: effectiveSelected,
                              onSelectCase: onSelectCase,
                              compact: false,
                            ),
                          ),
                        ),
                        const SizedBox(width: 14),
                        Expanded(
                          child: effectiveSelected == null
                              ? const _EmptyWorkspace()
                              : AgentWorkspacePanel(
                                  controller: controller,
                                  caseId: effectiveSelected,
                                ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            );
          },
        );
      },
    );
  }

  String? _effectiveSelected(List<SupportCase> items) {
    if (items.isEmpty) {
      return null;
    }
    if (selectedCaseId != null &&
        items.any((SupportCase item) => item.id == selectedCaseId)) {
      return selectedCaseId;
    }
    return items.first.id;
  }
}

class _CasesHeader extends StatelessWidget {
  const _CasesHeader({required this.mode});

  final _CasesMode mode;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.end,
      children: <Widget>[
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text(
                mode == _CasesMode.waiting
                    ? 'Fila de atendimento'
                    : 'Atendimentos em andamento',
                style: Theme.of(context).textTheme.headlineMedium,
              ),
              const SizedBox(height: 5),
              Text(
                mode == _CasesMode.waiting
                    ? 'Eventos de baixa confiança aguardando uma pessoa atendente.'
                    : 'Conversas assumidas com histórico e resumo contextual.',
                style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                      color: AppColors.muted,
                    ),
              ),
            ],
          ),
        ),
        const SizedBox(width: 12),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
          decoration: BoxDecoration(
            color: const Color(0xFFE8F6EF),
            borderRadius: BorderRadius.circular(999),
          ),
          child: const Row(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              Icon(Icons.circle, size: 8, color: AppColors.success),
              SizedBox(width: 7),
              Text(
                'Tempo real',
                style: TextStyle(
                  color: AppColors.success,
                  fontSize: 11,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _MetricsGrid extends StatelessWidget {
  const _MetricsGrid({required this.controller});

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (BuildContext context, BoxConstraints constraints) {
        final int columns = constraints.maxWidth >= 980 ? 4 : 2;
        final double gap = 10;
        final double cardWidth =
            (constraints.maxWidth - (gap * (columns - 1))) / columns;

        final List<Widget> metrics = <Widget>[
          MetricCard(
            label: 'Na fila',
            value: '${controller.waitingCases.length}',
            icon: Icons.inbox_outlined,
            accent: AppColors.warning,
            helper: 'Requer humano',
          ),
          MetricCard(
            label: 'Em atendimento',
            value: '${controller.activeCases.length}',
            icon: Icons.support_agent_rounded,
            accent: AppColors.purple,
            helper: 'Sessões assumidas',
          ),
          MetricCard(
            label: 'Concluídos',
            value: '${controller.resolvedCases.length}',
            icon: Icons.task_alt_rounded,
            accent: AppColors.success,
            helper: 'Nesta demonstração',
          ),
          MetricCard(
            label: 'Eventos não lidos',
            value: '${controller.unattendedEvents}',
            icon: Icons.notifications_active_outlined,
            accent: AppColors.claroRed,
            helper: 'Kafka / WebSocket',
          ),
        ];

        return Wrap(
          spacing: gap,
          runSpacing: gap,
          children: metrics
              .map((Widget metric) => SizedBox(width: cardWidth, child: metric))
              .toList(),
        );
      },
    );
  }
}

class _LiveEventBanner extends StatelessWidget {
  const _LiveEventBanner({required this.controller});

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(15, 11, 8, 11),
      decoration: BoxDecoration(
        color: const Color(0xFFFFF3E7),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: const Color(0xFFFFD4AA)),
      ),
      child: Row(
        children: <Widget>[
          const Icon(
            Icons.notifications_active_rounded,
            color: AppColors.warning,
            size: 21,
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              '${controller.unattendedEvents} novo(s) cliente(s) na fila • evento REQUIRED_HUMAN_ASSISTANCE',
              style: const TextStyle(
                color: AppColors.warning,
                fontSize: 12,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
          IconButton(
            tooltip: 'Ocultar alerta',
            onPressed: controller.dismissLiveAlert,
            icon: const Icon(Icons.close_rounded, size: 19),
          ),
        ],
      ),
    );
  }
}

class _CasesListContent extends StatelessWidget {
  const _CasesListContent({
    required this.controller,
    required this.items,
    required this.selectedCaseId,
    required this.onSelectCase,
    required this.compact,
  });

  final OrionController controller;
  final List<SupportCase> items;
  final String? selectedCaseId;
  final void Function(BuildContext context, String caseId) onSelectCase;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    if (items.isEmpty) {
      return SurfaceCard(
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 32),
          child: Column(
            children: <Widget>[
              const Icon(
                Icons.inbox_outlined,
                size: 44,
                color: AppColors.muted,
              ),
              const SizedBox(height: 12),
              Text(
                'Nenhum atendimento nesta lista',
                style: Theme.of(context).textTheme.titleMedium,
              ),
              const SizedBox(height: 5),
              Text(
                'Novos eventos aparecerão aqui automaticamente.',
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
          ),
        ),
      );
    }

    final Widget list = ListView.separated(
      shrinkWrap: compact,
      physics: compact
          ? const NeverScrollableScrollPhysics()
          : const AlwaysScrollableScrollPhysics(),
      padding: compact
          ? EdgeInsets.zero
          : const EdgeInsets.fromLTRB(10, 10, 10, 10),
      itemCount: items.length,
      separatorBuilder: (BuildContext context, int index) =>
          const SizedBox(height: 9),
      itemBuilder: (BuildContext context, int index) {
        final SupportCase item = items[index];
        return _CaseTile(
          item: item,
          selected: selectedCaseId == item.id,
          onTap: () => onSelectCase(context, item.id),
          onTake: item.status == SupportCaseStatus.waitingHuman
              ? () => controller.takeCase(item.id)
              : null,
        );
      },
    );

    if (compact) {
      return list;
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 15, 16, 8),
          child: Row(
            children: <Widget>[
              Expanded(
                child: Text(
                  'Conversas (${items.length})',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
              ),
              const Icon(Icons.filter_list_rounded, color: AppColors.muted),
            ],
          ),
        ),
        const Divider(height: 1),
        Expanded(child: list),
      ],
    );
  }
}

class _CaseTile extends StatelessWidget {
  const _CaseTile({
    required this.item,
    required this.selected,
    required this.onTap,
    this.onTake,
  });

  final SupportCase item;
  final bool selected;
  final VoidCallback onTap;
  final VoidCallback? onTake;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      onTap: onTap,
      padding: const EdgeInsets.all(14),
      color: selected ? const Color(0xFFFFF6F7) : AppColors.surface,
      borderColor: selected ? const Color(0xFFFFB8BE) : AppColors.border,
      radius: 17,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Stack(
                clipBehavior: Clip.none,
                children: <Widget>[
                  CircleAvatar(
                    radius: 21,
                    backgroundColor: const Color(0xFFEDEEF2),
                    child: Text(
                      _initials(item.customerName),
                      style: const TextStyle(
                        color: AppColors.ink,
                        fontWeight: FontWeight.w900,
                        fontSize: 12,
                      ),
                    ),
                  ),
                  if (item.hasUnreadEvent)
                    const Positioned(
                      right: -1,
                      top: -1,
                      child: CircleAvatar(
                        radius: 6,
                        backgroundColor: AppColors.claroRed,
                      ),
                    ),
                ],
              ),
              const SizedBox(width: 11),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    Text(
                      item.customerName,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    Text(
                      '${item.id} • ${item.lastChannel.label}',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
              Text(
                _relativeTime(item.updatedAt),
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
          ),
          const SizedBox(height: 11),
          Text(
            item.lastMessage?.text ?? item.summary,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: AppColors.ink,
                ),
          ),
          const SizedBox(height: 11),
          Wrap(
            spacing: 7,
            runSpacing: 7,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: <Widget>[
              StatusPill(status: item.status, compact: true),
              Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
                decoration: BoxDecoration(
                  color: AppColors.canvas,
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  '${(item.intentConfidence * 100).round()}% confiança',
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        fontWeight: FontWeight.w800,
                      ),
                ),
              ),
            ],
          ),
          if (onTake != null) ...<Widget>[
            const SizedBox(height: 11),
            SizedBox(
              width: double.infinity,
              child: OutlinedButton.icon(
                onPressed: onTake,
                icon: const Icon(Icons.person_add_alt_1_rounded, size: 18),
                label: const Text('Assumir atendimento'),
              ),
            ),
          ],
        ],
      ),
    );
  }

  static String _initials(String name) {
    final List<String> parts = name
        .split(' ')
        .where((String part) => part.trim().isNotEmpty)
        .toList();
    if (parts.isEmpty) {
      return 'CX';
    }
    if (parts.length == 1) {
      return parts.first.substring(0, 1).toUpperCase();
    }
    return '${parts.first.substring(0, 1)}${parts.last.substring(0, 1)}'
        .toUpperCase();
  }

  static String _relativeTime(DateTime value) {
    final Duration difference = DateTime.now().difference(value);
    if (difference.inMinutes < 1) {
      return 'agora';
    }
    if (difference.inMinutes < 60) {
      return '${difference.inMinutes} min';
    }
    return '${difference.inHours} h';
  }
}

class AgentWorkspaceScreen extends StatelessWidget {
  const AgentWorkspaceScreen({
    required this.controller,
    required this.caseId,
    super.key,
  });

  final OrionController controller;
  final String caseId;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Atendimento'),
      ),
      body: Padding(
        padding: const EdgeInsets.all(12),
        child: AgentWorkspacePanel(
          controller: controller,
          caseId: caseId,
        ),
      ),
    );
  }
}

class AgentWorkspacePanel extends StatefulWidget {
  const AgentWorkspacePanel({
    required this.controller,
    required this.caseId,
    super.key,
  });

  final OrionController controller;
  final String caseId;

  @override
  State<AgentWorkspacePanel> createState() => _AgentWorkspacePanelState();
}

class _AgentWorkspacePanelState extends State<AgentWorkspacePanel> {
  final TextEditingController _replyController = TextEditingController();
  final ScrollController _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_scrollToBottom);
  }

  @override
  void didUpdateWidget(covariant AgentWorkspacePanel oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.caseId != widget.caseId) {
      _replyController.clear();
      _scrollToBottom();
    }
  }

  @override
  void dispose() {
    widget.controller.removeListener(_scrollToBottom);
    _replyController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  void _scrollToBottom() {
    if (!mounted) {
      return;
    }
    WidgetsBinding.instance.addPostFrameCallback((Duration timeStamp) {
      if (!_scrollController.hasClients) {
        return;
      }
      _scrollController.animateTo(
        _scrollController.position.maxScrollExtent,
        duration: const Duration(milliseconds: 220),
        curve: Curves.easeOut,
      );
    });
  }

  void _sendReply() {
    final String text = _replyController.text;
    if (text.trim().isEmpty) {
      return;
    }
    widget.controller.sendAgentMessage(widget.caseId, text);
    _replyController.clear();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (BuildContext context, Widget? child) {
        final SupportCase item = widget.controller.caseById(widget.caseId);

        return SurfaceCard(
          padding: EdgeInsets.zero,
          child: LayoutBuilder(
            builder: (BuildContext context, BoxConstraints constraints) {
              final bool showSidebar = constraints.maxWidth >= 820;
              return Column(
                children: <Widget>[
                  _WorkspaceHeader(
                    controller: widget.controller,
                    item: item,
                    showDetailsButton: !showSidebar,
                  ),
                  const Divider(height: 1),
                  Expanded(
                    child: showSidebar
                        ? Row(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: <Widget>[
                              Expanded(
                                child: _ConversationWorkspace(
                                  item: item,
                                  replyController: _replyController,
                                  scrollController: _scrollController,
                                  onSend: _sendReply,
                                ),
                              ),
                              const VerticalDivider(width: 1),
                              SizedBox(
                                width: 305,
                                child: _CaseDetailsPanel(item: item),
                              ),
                            ],
                          )
                        : _ConversationWorkspace(
                            item: item,
                            replyController: _replyController,
                            scrollController: _scrollController,
                            onSend: _sendReply,
                          ),
                  ),
                ],
              );
            },
          ),
        );
      },
    );
  }
}

class _WorkspaceHeader extends StatelessWidget {
  const _WorkspaceHeader({
    required this.controller,
    required this.item,
    required this.showDetailsButton,
  });

  final OrionController controller;
  final SupportCase item;
  final bool showDetailsButton;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 13, 12, 13),
      child: Row(
        children: <Widget>[
          CircleAvatar(
            radius: 21,
            backgroundColor: const Color(0xFFEDEEF2),
            child: Text(
              _CaseTile._initials(item.customerName),
              style: const TextStyle(
                color: AppColors.ink,
                fontWeight: FontWeight.w900,
                fontSize: 12,
              ),
            ),
          ),
          const SizedBox(width: 11),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  item.customerName,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                Text(
                  '${item.id} • ${item.lastChannel.label}',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ],
            ),
          ),
          if (showDetailsButton)
            IconButton(
              tooltip: 'Ver contexto',
              onPressed: () {
                showModalBottomSheet<void>(
                  context: context,
                  isScrollControlled: true,
                  showDragHandle: true,
                  builder: (BuildContext context) => FractionallySizedBox(
                    heightFactor: 0.82,
                    child: _CaseDetailsPanel(item: item),
                  ),
                );
              },
              icon: const Icon(Icons.info_outline_rounded),
            ),
          if (item.status == SupportCaseStatus.waitingHuman)
            FilledButton.icon(
              onPressed: () => controller.takeCase(item.id),
              icon: const Icon(Icons.person_add_alt_1_rounded, size: 18),
              label: const Text('Assumir'),
            )
          else if (item.status == SupportCaseStatus.inProgress)
            OutlinedButton.icon(
              onPressed: () => controller.resolveCase(item.id),
              icon: const Icon(Icons.task_alt_rounded, size: 18),
              label: const Text('Concluir'),
            )
          else
            StatusPill(status: item.status, compact: true),
        ],
      ),
    );
  }
}

class _ConversationWorkspace extends StatelessWidget {
  const _ConversationWorkspace({
    required this.item,
    required this.replyController,
    required this.scrollController,
    required this.onSend,
  });

  final SupportCase item;
  final TextEditingController replyController;
  final ScrollController scrollController;
  final VoidCallback onSend;

  @override
  Widget build(BuildContext context) {
    final bool canReply = item.status == SupportCaseStatus.inProgress;

    return Column(
      children: <Widget>[
        Container(
          width: double.infinity,
          padding: const EdgeInsets.fromLTRB(14, 11, 14, 11),
          color: const Color(0xFFF8F5FF),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              const Icon(
                Icons.auto_awesome_rounded,
                size: 18,
                color: AppColors.purple,
              ),
              const SizedBox(width: 9),
              Expanded(
                child: Text(
                  'Resumo da IA: ${item.summary}',
                  maxLines: 3,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: AppColors.ink,
                      ),
                ),
              ),
            ],
          ),
        ),
        Expanded(
          child: ListView.builder(
            controller: scrollController,
            padding: const EdgeInsets.fromLTRB(16, 13, 16, 12),
            itemCount: item.messages.length,
            itemBuilder: (BuildContext context, int index) {
              return ChatBubble(message: item.messages[index]);
            },
          ),
        ),
        const Divider(height: 1),
        if (!canReply)
          Padding(
            padding: const EdgeInsets.all(13),
            child: Container(
              width: double.infinity,
              padding: const EdgeInsets.all(13),
              decoration: BoxDecoration(
                color: item.status == SupportCaseStatus.resolved
                    ? const Color(0xFFEAF7F0)
                    : const Color(0xFFFFF3E7),
                borderRadius: BorderRadius.circular(14),
              ),
              child: Text(
                item.status == SupportCaseStatus.resolved
                    ? 'Atendimento concluído. O histórico está disponível para consulta.'
                    : 'Assuma o atendimento para habilitar a resposta manual.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: item.status == SupportCaseStatus.resolved
                      ? AppColors.success
                      : AppColors.warning,
                  fontSize: 12,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ),
          )
        else
          Padding(
            padding: const EdgeInsets.all(12),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: <Widget>[
                Expanded(
                  child: TextField(
                    controller: replyController,
                    minLines: 1,
                    maxLines: 4,
                    textInputAction: TextInputAction.send,
                    onSubmitted: (String value) => onSend(),
                    decoration: const InputDecoration(
                      hintText: 'Digite a resposta para o cliente',
                      prefixIcon: Icon(Icons.edit_outlined),
                    ),
                  ),
                ),
                const SizedBox(width: 9),
                SizedBox(
                  width: 50,
                  height: 50,
                  child: FilledButton(
                    onPressed: onSend,
                    style: FilledButton.styleFrom(padding: EdgeInsets.zero),
                    child: const Icon(Icons.send_rounded),
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }
}

class _CaseDetailsPanel extends StatelessWidget {
  const _CaseDetailsPanel({required this.item});

  final SupportCase item;

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(17),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Text('Contexto do cliente',
              style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 15),
          _DetailRow(label: 'Cliente', value: item.customerName),
          _DetailRow(label: 'Documento', value: item.customerDocument),
          _DetailRow(label: 'Plano', value: item.planName),
          _DetailRow(label: 'Protocolo', value: item.id),
          _DetailRow(label: 'Canal', value: item.lastChannel.label),
          const Divider(height: 26),
          Text('Classificação',
              style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 11),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(11),
            decoration: BoxDecoration(
              color: AppColors.canvas,
              borderRadius: BorderRadius.circular(13),
            ),
            child: Text(
              item.intent,
              style: const TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 12,
              ),
            ),
          ),
          const SizedBox(height: 12),
          ConfidenceBar(value: item.intentConfidence),
          const SizedBox(height: 18),
          Text('Resumo contextual',
              style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 7),
          Text(item.summary, style: Theme.of(context).textTheme.bodySmall),
          const Divider(height: 28),
          Text('Linha do atendimento',
              style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 12),
          const _TimelineItem(
            icon: Icons.message_outlined,
            title: 'Mensagem recebida',
            subtitle: 'Canal conectado ao ORION Gateway',
            done: true,
          ),
          const _TimelineItem(
            icon: Icons.psychology_alt_outlined,
            title: 'Intenção analisada',
            subtitle: 'Motor NLP / IA gerou confiança e resumo',
            done: true,
          ),
          _TimelineItem(
            icon: Icons.support_agent_rounded,
            title: 'Atendimento humano',
            subtitle: item.status == SupportCaseStatus.waitingHuman
                ? 'Aguardando alguém assumir'
                : item.assignedAgent ?? 'Atendimento concluído',
            done: item.status != SupportCaseStatus.waitingHuman,
          ),
        ],
      ),
    );
  }
}

class _DetailRow extends StatelessWidget {
  const _DetailRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Text(label, style: Theme.of(context).textTheme.bodySmall),
          const SizedBox(height: 2),
          Text(
            value,
            style: const TextStyle(
              color: AppColors.ink,
              fontSize: 13,
              fontWeight: FontWeight.w800,
            ),
          ),
        ],
      ),
    );
  }
}

class _TimelineItem extends StatelessWidget {
  const _TimelineItem({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.done,
  });

  final IconData icon;
  final String title;
  final String subtitle;
  final bool done;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 13),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Container(
            width: 31,
            height: 31,
            decoration: BoxDecoration(
              color: done
                  ? AppColors.success.withValues(alpha: 0.10)
                  : AppColors.canvas,
              shape: BoxShape.circle,
            ),
            child: Icon(
              icon,
              size: 16,
              color: done ? AppColors.success : AppColors.muted,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  title,
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 2),
                Text(subtitle, style: Theme.of(context).textTheme.bodySmall),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _EmptyWorkspace extends StatelessWidget {
  const _EmptyWorkspace();

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      child: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            const Icon(
              Icons.forum_outlined,
              size: 52,
              color: AppColors.muted,
            ),
            const SizedBox(height: 13),
            Text(
              'Selecione uma conversa',
              style: Theme.of(context).textTheme.titleLarge,
            ),
            const SizedBox(height: 5),
            Text(
              'O histórico e o resumo contextual aparecerão aqui.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
        ),
      ),
    );
  }
}

class _MonitoringPage extends StatelessWidget {
  const _MonitoringPage({required this.controller});

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (BuildContext context, Widget? child) {
        return SingleChildScrollView(
          child: PageContainer(
            maxWidth: 1220,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Row(
                  crossAxisAlignment: CrossAxisAlignment.end,
                  children: <Widget>[
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: <Widget>[
                          Text(
                            'Monitoramento da solução',
                            style: Theme.of(context).textTheme.headlineMedium,
                          ),
                          const SizedBox(height: 5),
                          Text(
                            'Visão demonstrativa dos componentes descritos na arquitetura Orion CX.',
                            style: Theme.of(context)
                                .textTheme
                                .bodyMedium
                                ?.copyWith(color: AppColors.muted),
                          ),
                        ],
                      ),
                    ),
                    OutlinedButton.icon(
                      onPressed: () {
                        ScaffoldMessenger.of(context).showSnackBar(
                          const SnackBar(
                            content: Text(
                              'Status simulado atualizado com sucesso.',
                            ),
                          ),
                        );
                      },
                      icon: const Icon(Icons.refresh_rounded),
                      label: const Text('Atualizar'),
                    ),
                  ],
                ),
                const SizedBox(height: 18),
                Container(
                  padding: const EdgeInsets.all(15),
                  decoration: BoxDecoration(
                    color: const Color(0xFFFFF8E8),
                    borderRadius: BorderRadius.circular(16),
                    border: Border.all(color: const Color(0xFFFFE2A3)),
                  ),
                  child: const Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Icon(Icons.science_outlined, color: AppColors.warning),
                      SizedBox(width: 11),
                      Expanded(
                        child: Text(
                          'Os indicadores desta tela são mocks de interface; não existe conexão real com AWS, Kubernetes ou Kafka neste protótipo.',
                          style: TextStyle(
                            color: AppColors.warning,
                            fontSize: 12,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 24),
                Text(
                  'Fluxo da arquitetura',
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                const SizedBox(height: 13),
                const _ArchitectureFlow(),
                const SizedBox(height: 26),
                Text(
                  'Serviços',
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                const SizedBox(height: 13),
                const _ServicesGrid(),
                const SizedBox(height: 26),
                Text(
                  'Eventos recentes',
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                const SizedBox(height: 13),
                SurfaceCard(
                  child: Column(
                    children: <Widget>[
                      _EventRow(
                        icon: Icons.notifications_active_outlined,
                        title: 'REQUIRED_HUMAN_ASSISTANCE',
                        subtitle:
                            '${controller.waitingCases.length} sessão(ões) aguardando atendimento humano',
                        accent: AppColors.warning,
                      ),
                      const Divider(height: 22),
                      _EventRow(
                        icon: Icons.support_agent_rounded,
                        title: 'CALL_ASSIGNED',
                        subtitle:
                            '${controller.activeCases.length} sessão(ões) em atendimento',
                        accent: AppColors.purple,
                      ),
                      const Divider(height: 22),
                      _EventRow(
                        icon: Icons.task_alt_rounded,
                        title: 'SESSION_RESOLVED',
                        subtitle:
                            '${controller.resolvedCases.length} sessão(ões) concluídas',
                        accent: AppColors.success,
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

class _ArchitectureFlow extends StatelessWidget {
  const _ArchitectureFlow();

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (BuildContext context, BoxConstraints constraints) {
        final bool wide = constraints.maxWidth >= 880;
        final List<Widget> nodes = <Widget>[
          const _ArchitectureNode(
            icon: Icons.devices_other_rounded,
            title: 'Canais',
            subtitle: 'App • Web • WhatsApp',
            accent: AppColors.claroRed,
          ),
          const _ArchitectureNode(
            icon: Icons.api_rounded,
            title: 'API Gateway',
            subtitle: 'Entrada REST / HTTPS',
            accent: AppColors.info,
          ),
          const _ArchitectureNode(
            icon: Icons.hub_outlined,
            title: 'Cluster EKS',
            subtitle: 'Gateway e microsserviços',
            accent: AppColors.purple,
          ),
          const _ArchitectureNode(
            icon: Icons.storage_rounded,
            title: 'Dados e eventos',
            subtitle: 'DynamoDB • RDS • S3 • Kafka',
            accent: AppColors.success,
          ),
        ];

        if (!wide) {
          return Column(
            children: <Widget>[
              nodes[0],
              const _FlowArrow(vertical: true),
              nodes[1],
              const _FlowArrow(vertical: true),
              nodes[2],
              const _FlowArrow(vertical: true),
              nodes[3],
            ],
          );
        }

        return Row(
          children: <Widget>[
            Expanded(child: nodes[0]),
            const _FlowArrow(),
            Expanded(child: nodes[1]),
            const _FlowArrow(),
            Expanded(child: nodes[2]),
            const _FlowArrow(),
            Expanded(child: nodes[3]),
          ],
        );
      },
    );
  }
}

class _ArchitectureNode extends StatelessWidget {
  const _ArchitectureNode({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.accent,
  });

  final IconData icon;
  final String title;
  final String subtitle;
  final Color accent;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        children: <Widget>[
          Container(
            width: 43,
            height: 43,
            decoration: BoxDecoration(
              color: accent.withValues(alpha: 0.10),
              borderRadius: BorderRadius.circular(14),
            ),
            child: Icon(icon, color: accent),
          ),
          const SizedBox(height: 10),
          Text(
            title,
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.titleMedium,
          ),
          const SizedBox(height: 4),
          Text(
            subtitle,
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    );
  }
}

class _FlowArrow extends StatelessWidget {
  const _FlowArrow({this.vertical = false});

  final bool vertical;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: vertical
          ? const EdgeInsets.symmetric(vertical: 7)
          : const EdgeInsets.symmetric(horizontal: 7),
      child: Icon(
        vertical ? Icons.arrow_downward_rounded : Icons.arrow_forward_rounded,
        color: AppColors.muted,
        size: 20,
      ),
    );
  }
}

class _ServicesGrid extends StatelessWidget {
  const _ServicesGrid();

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (BuildContext context, BoxConstraints constraints) {
        final int columns = constraints.maxWidth >= 1050
            ? 4
            : constraints.maxWidth >= 650
                ? 2
                : 1;
        final double gap = 11;
        final double itemWidth =
            (constraints.maxWidth - (gap * (columns - 1))) / columns;
        const List<_ServiceData> services = <_ServiceData>[
          _ServiceData(
            name: 'ORION Gateway',
            description: 'Orquestra sessão e chamadas gRPC',
            icon: Icons.route_outlined,
          ),
          _ServiceData(
            name: 'ORION Motor NLP / IA',
            description: 'Classifica intenção e confiança',
            icon: Icons.psychology_alt_outlined,
          ),
          _ServiceData(
            name: 'ORION Authenticator',
            description: 'Autenticação e segurança',
            icon: Icons.verified_user_outlined,
          ),
          _ServiceData(
            name: 'ORION Call Management',
            description: 'Ciclo de vida dos atendimentos',
            icon: Icons.support_agent_rounded,
          ),
          _ServiceData(
            name: 'ORION Notification',
            description: 'Integração de notificações',
            icon: Icons.notifications_none_rounded,
          ),
          _ServiceData(
            name: 'Apache Kafka',
            description: 'Barramento assíncrono de eventos',
            icon: Icons.swap_horiz_rounded,
          ),
          _ServiceData(
            name: 'DynamoDB / RDS',
            description: 'Contexto e dados persistentes',
            icon: Icons.storage_rounded,
          ),
          _ServiceData(
            name: 'Amazon S3 / SNS',
            description: 'Arquivos e mensageria de saída',
            icon: Icons.cloud_outlined,
          ),
        ];

        return Wrap(
          spacing: gap,
          runSpacing: gap,
          children: services
              .map(
                (_ServiceData service) => SizedBox(
                  width: itemWidth,
                  child: _ServiceCard(data: service),
                ),
              )
              .toList(),
        );
      },
    );
  }
}

class _ServiceData {
  const _ServiceData({
    required this.name,
    required this.description,
    required this.icon,
  });

  final String name;
  final String description;
  final IconData icon;
}

class _ServiceCard extends StatelessWidget {
  const _ServiceCard({required this.data});

  final _ServiceData data;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.all(15),
      child: Row(
        children: <Widget>[
          Container(
            width: 42,
            height: 42,
            decoration: BoxDecoration(
              color: AppColors.canvas,
              borderRadius: BorderRadius.circular(13),
            ),
            child: Icon(data.icon, color: AppColors.ink, size: 21),
          ),
          const SizedBox(width: 11),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  data.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w900,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  data.description,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ],
            ),
          ),
          const SizedBox(width: 7),
          const Icon(Icons.circle, color: AppColors.success, size: 9),
        ],
      ),
    );
  }
}

class _EventRow extends StatelessWidget {
  const _EventRow({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.accent,
  });

  final IconData icon;
  final String title;
  final String subtitle;
  final Color accent;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: <Widget>[
        Container(
          width: 39,
          height: 39,
          decoration: BoxDecoration(
            color: accent.withValues(alpha: 0.10),
            shape: BoxShape.circle,
          ),
          child: Icon(icon, color: accent, size: 19),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text(
                title,
                style: const TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w900,
                ),
              ),
              const SizedBox(height: 3),
              Text(subtitle, style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ),
        Text(
          'agora',
          style: Theme.of(context).textTheme.bodySmall,
        ),
      ],
    );
  }
}
