import 'package:flutter/material.dart';

import '../core/app_theme.dart';
import '../core/models.dart';
import '../core/orion_controller.dart';
import '../widgets/common_widgets.dart';

class CustomerPortal extends StatefulWidget {
  const CustomerPortal({
    required this.controller,
    super.key,
  });

  final OrionController controller;

  @override
  State<CustomerPortal> createState() => _CustomerPortalState();
}

class _CustomerPortalState extends State<CustomerPortal> {
  int _selectedIndex = 0;

  void _openChat() {
    setState(() => _selectedIndex = 1);
  }

  void _startScenario(String message) {
    setState(() => _selectedIndex = 1);
    widget.controller.sendCustomerMessage(message);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const BrandMark(compact: true),
        actions: <Widget>[
          IconButton(
            tooltip: 'Novo atendimento',
            onPressed: widget.controller.resetCustomerConversation,
            icon: const Icon(Icons.add_comment_outlined),
          ),
          IconButton(
            tooltip: 'Sair da área do cliente',
            onPressed: () => Navigator.of(context).pop(),
            icon: const Icon(Icons.logout_rounded),
          ),
          const SizedBox(width: 8),
        ],
      ),
      body: IndexedStack(
        index: _selectedIndex,
        children: <Widget>[
          _CustomerHomeTab(
            controller: widget.controller,
            onOpenChat: _openChat,
            onStartScenario: _startScenario,
          ),
          _CustomerChatTab(controller: widget.controller),
          _CustomerSessionsTab(
            controller: widget.controller,
            onOpenChat: _openChat,
          ),
        ],
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _selectedIndex,
        onDestinationSelected: (int index) {
          setState(() => _selectedIndex = index);
        },
        destinations: const <NavigationDestination>[
          NavigationDestination(
            icon: Icon(Icons.home_outlined),
            selectedIcon: Icon(Icons.home_rounded),
            label: 'Início',
          ),
          NavigationDestination(
            icon: Icon(Icons.chat_bubble_outline_rounded),
            selectedIcon: Icon(Icons.chat_bubble_rounded),
            label: 'Atendimento',
          ),
          NavigationDestination(
            icon: Icon(Icons.history_rounded),
            selectedIcon: Icon(Icons.manage_history_rounded),
            label: 'Sessões',
          ),
        ],
      ),
    );
  }
}

class _CustomerHomeTab extends StatelessWidget {
  const _CustomerHomeTab({
    required this.controller,
    required this.onOpenChat,
    required this.onStartScenario,
  });

  final OrionController controller;
  final VoidCallback onOpenChat;
  final ValueChanged<String> onStartScenario;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (BuildContext context, Widget? child) {
        final SupportCase item = controller.customerCase;
        return SingleChildScrollView(
          child: PageContainer(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(26),
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(26),
                    gradient: const LinearGradient(
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                      colors: <Color>[
                        AppColors.claroRed,
                        AppColors.claroDarkRed,
                      ],
                    ),
                  ),
                  child: LayoutBuilder(
                    builder:
                        (BuildContext context, BoxConstraints constraints) {
                      final bool wide = constraints.maxWidth >= 720;
                      final Widget copy = Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: <Widget>[
                          const Text(
                            'Olá, Cliente Demo',
                            style: TextStyle(
                              color: Colors.white70,
                              fontSize: 14,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            'Como podemos ajudar hoje?',
                            style: Theme.of(context)
                                .textTheme
                                .headlineMedium
                                ?.copyWith(color: Colors.white),
                          ),
                          const SizedBox(height: 10),
                          const Text(
                            'Comece pelo assunto e o Orion mantém o contexto durante toda a jornada.',
                            style: TextStyle(
                              color: Colors.white70,
                              height: 1.5,
                            ),
                          ),
                          const SizedBox(height: 20),
                          FilledButton.icon(
                            onPressed: onOpenChat,
                            style: FilledButton.styleFrom(
                              backgroundColor: Colors.white,
                              foregroundColor: AppColors.claroRed,
                            ),
                            icon: const Icon(Icons.chat_bubble_outline_rounded),
                            label: const Text('Abrir atendimento'),
                          ),
                        ],
                      );

                      if (!wide) {
                        return copy;
                      }

                      return Row(
                        children: <Widget>[
                          Expanded(child: copy),
                          const SizedBox(width: 30),
                          Container(
                            width: 230,
                            padding: const EdgeInsets.all(18),
                            decoration: BoxDecoration(
                              color: Colors.white.withValues(alpha: 0.10),
                              borderRadius: BorderRadius.circular(20),
                              border: Border.all(color: Colors.white24),
                            ),
                            child: const Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: <Widget>[
                                Icon(
                                  Icons.cloud_done_outlined,
                                  color: Colors.white,
                                  size: 30,
                                ),
                                SizedBox(height: 12),
                                Text(
                                  'Contexto sincronizado',
                                  style: TextStyle(
                                    color: Colors.white,
                                    fontWeight: FontWeight.w800,
                                  ),
                                ),
                                SizedBox(height: 6),
                                Text(
                                  'App, Web e WhatsApp podem continuar a mesma sessão.',
                                  style: TextStyle(
                                    color: Colors.white70,
                                    fontSize: 12,
                                    height: 1.4,
                                  ),
                                ),
                              ],
                            ),
                          ),
                        ],
                      );
                    },
                  ),
                ),
                const SizedBox(height: 28),
                Text(
                  'Atalhos de atendimento',
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                const SizedBox(height: 6),
                Text(
                  'Selecione um cenário para executar o fluxo completo.',
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        color: AppColors.muted,
                      ),
                ),
                const SizedBox(height: 16),
                LayoutBuilder(
                  builder: (BuildContext context, BoxConstraints constraints) {
                    final bool wide = constraints.maxWidth >= 680;
                    final List<Widget> cards = <Widget>[
                      _QuickSupportCard(
                        icon: Icons.wifi_tethering_error_rounded,
                        title: 'Internet lenta',
                        description:
                            'Análise automática, confirmação e reinício simulado do sinal.',
                        accent: AppColors.info,
                        onTap: () => onStartScenario(
                          'Minha internet está lenta.',
                        ),
                      ),
                      _QuickSupportCard(
                        icon: Icons.receipt_long_outlined,
                        title: 'Cobrança indevida',
                        description:
                            'Classificação com baixa confiança e transferência para atendente.',
                        accent: AppColors.warning,
                        onTap: () => onStartScenario(
                          'Quero contestar uma cobrança indevida na minha fatura.',
                        ),
                      ),
                    ];

                    if (!wide) {
                      return Column(
                        children: <Widget>[
                          cards[0],
                          const SizedBox(height: 13),
                          cards[1],
                        ],
                      );
                    }

                    return Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: <Widget>[
                        Expanded(child: cards[0]),
                        const SizedBox(width: 14),
                        Expanded(child: cards[1]),
                      ],
                    );
                  },
                ),
                const SizedBox(height: 28),
                Text(
                  'Sessão atual',
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                const SizedBox(height: 14),
                SurfaceCard(
                  onTap: onOpenChat,
                  child: LayoutBuilder(
                    builder:
                        (BuildContext context, BoxConstraints constraints) {
                      final bool wide = constraints.maxWidth >= 680;
                      final Widget details = Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: <Widget>[
                          Wrap(
                            spacing: 9,
                            runSpacing: 9,
                            crossAxisAlignment: WrapCrossAlignment.center,
                            children: <Widget>[
                              Text(
                                item.id,
                                style: Theme.of(context).textTheme.titleLarge,
                              ),
                              StatusPill(status: item.status, compact: true),
                            ],
                          ),
                          const SizedBox(height: 8),
                          Text(
                            item.summary,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: Theme.of(context)
                                .textTheme
                                .bodyMedium
                                ?.copyWith(color: AppColors.muted),
                          ),
                          const SizedBox(height: 12),
                          Row(
                            children: <Widget>[
                              const Icon(
                                Icons.devices_rounded,
                                size: 17,
                                color: AppColors.muted,
                              ),
                              const SizedBox(width: 7),
                              Text(
                                'Último canal: ${controller.currentChannel.label}',
                                style: Theme.of(context).textTheme.bodySmall,
                              ),
                            ],
                          ),
                        ],
                      );

                      if (!wide) {
                        return Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: <Widget>[
                            details,
                            const SizedBox(height: 16),
                            const Row(
                              mainAxisAlignment: MainAxisAlignment.end,
                              children: <Widget>[
                                Text(
                                  'Continuar',
                                  style: TextStyle(fontWeight: FontWeight.w800),
                                ),
                                SizedBox(width: 5),
                                Icon(Icons.arrow_forward_rounded, size: 19),
                              ],
                            ),
                          ],
                        );
                      }

                      return Row(
                        children: <Widget>[
                          Expanded(child: details),
                          const SizedBox(width: 24),
                          const Icon(
                            Icons.arrow_forward_rounded,
                            color: AppColors.claroRed,
                          ),
                        ],
                      );
                    },
                  ),
                ),
                const SizedBox(height: 28),
                Text(
                  'Como a continuidade funciona',
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                const SizedBox(height: 14),
                const _ContinuitySteps(),
              ],
            ),
          ),
        );
      },
    );
  }
}

class _QuickSupportCard extends StatelessWidget {
  const _QuickSupportCard({
    required this.icon,
    required this.title,
    required this.description,
    required this.accent,
    required this.onTap,
  });

  final IconData icon;
  final String title;
  final String description;
  final Color accent;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      onTap: onTap,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Container(
            width: 50,
            height: 50,
            decoration: BoxDecoration(
              color: accent.withValues(alpha: 0.10),
              borderRadius: BorderRadius.circular(16),
            ),
            child: Icon(icon, color: accent),
          ),
          const SizedBox(width: 15),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(title, style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 5),
                Text(
                  description,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
                const SizedBox(height: 12),
                Text(
                  'Iniciar fluxo',
                  style: TextStyle(
                    color: accent,
                    fontSize: 12,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _ContinuitySteps extends StatelessWidget {
  const _ContinuitySteps();

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (BuildContext context, BoxConstraints constraints) {
        final bool wide = constraints.maxWidth >= 760;
        final List<Widget> steps = <Widget>[
          const _StepCard(
            number: '1',
            title: 'Mensagem recebida',
            description: 'O canal envia a solicitação para a sessão Orion.',
          ),
          const _StepCard(
            number: '2',
            title: 'Contexto persistido',
            description: 'Intenção e ação pendente ficam disponíveis na sessão.',
          ),
          const _StepCard(
            number: '3',
            title: 'Retomada em outro canal',
            description: 'O cliente continua sem repetir o histórico.',
          ),
        ];

        if (!wide) {
          return Column(
            children: <Widget>[
              steps[0],
              const SizedBox(height: 10),
              steps[1],
              const SizedBox(height: 10),
              steps[2],
            ],
          );
        }

        return Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Expanded(child: steps[0]),
            const SizedBox(width: 10),
            Expanded(child: steps[1]),
            const SizedBox(width: 10),
            Expanded(child: steps[2]),
          ],
        );
      },
    );
  }
}

class _StepCard extends StatelessWidget {
  const _StepCard({
    required this.number,
    required this.title,
    required this.description,
  });

  final String number;
  final String title;
  final String description;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.all(16),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Container(
            width: 34,
            height: 34,
            alignment: Alignment.center,
            decoration: const BoxDecoration(
              color: AppColors.ink,
              shape: BoxShape.circle,
            ),
            child: Text(
              number,
              style: const TextStyle(
                color: Colors.white,
                fontWeight: FontWeight.w900,
              ),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(title, style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 4),
                Text(description, style: Theme.of(context).textTheme.bodySmall),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _CustomerChatTab extends StatefulWidget {
  const _CustomerChatTab({required this.controller});

  final OrionController controller;

  @override
  State<_CustomerChatTab> createState() => _CustomerChatTabState();
}

class _CustomerChatTabState extends State<_CustomerChatTab> {
  final TextEditingController _inputController = TextEditingController();
  final ScrollController _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_scrollToBottom);
  }

  @override
  void dispose() {
    widget.controller.removeListener(_scrollToBottom);
    _inputController.dispose();
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
        duration: const Duration(milliseconds: 260),
        curve: Curves.easeOut,
      );
    });
  }

  void _sendMessage() {
    final String text = _inputController.text;
    if (text.trim().isEmpty) {
      return;
    }
    _inputController.clear();
    widget.controller.sendCustomerMessage(text);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (BuildContext context, Widget? child) {
        final SupportCase item = widget.controller.customerCase;
        return LayoutBuilder(
          builder: (BuildContext context, BoxConstraints constraints) {
            final double width =
                constraints.maxWidth > 1280 ? 1280 : constraints.maxWidth;
            final bool wide = width >= 940;

            return Center(
              child: SizedBox(
                width: width,
                height: constraints.maxHeight,
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(16, 14, 16, 16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: <Widget>[
                      _ChatToolbar(controller: widget.controller),
                      const SizedBox(height: 12),
                      Expanded(
                        child: wide
                            ? Row(
                                crossAxisAlignment: CrossAxisAlignment.stretch,
                                children: <Widget>[
                                  Expanded(child: _buildChatPanel(context, item)),
                                  const SizedBox(width: 14),
                                  SizedBox(
                                    width: 320,
                                    child: _CustomerContextPanel(
                                      controller: widget.controller,
                                      item: item,
                                    ),
                                  ),
                                ],
                              )
                            : _buildChatPanel(context, item),
                      ),
                    ],
                  ),
                ),
              ),
            );
          },
        );
      },
    );
  }

  Widget _buildChatPanel(BuildContext context, SupportCase item) {
    final bool inputEnabled =
        item.status != SupportCaseStatus.resolved && !widget.controller.isBotTyping;

    return SurfaceCard(
      padding: EdgeInsets.zero,
      child: Column(
        children: <Widget>[
          Padding(
            padding: const EdgeInsets.fromLTRB(18, 15, 18, 14),
            child: LayoutBuilder(
              builder: (BuildContext context, BoxConstraints constraints) {
                final bool showStatus = constraints.maxWidth >= 520;
                return Row(
                  children: <Widget>[
                    Container(
                      width: 39,
                      height: 39,
                      decoration: const BoxDecoration(
                        color: Color(0xFFFFE3E6),
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(
                        Icons.auto_awesome_rounded,
                        color: AppColors.claroRed,
                        size: 20,
                      ),
                    ),
                    const SizedBox(width: 11),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: <Widget>[
                          Text(
                            'Atendimento Orion',
                            style: Theme.of(context).textTheme.titleMedium,
                          ),
                          Text(
                            '${item.id} • ${widget.controller.currentChannel.label}',
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                        ],
                      ),
                    ),
                    if (showStatus) ...<Widget>[
                      const SizedBox(width: 10),
                      StatusPill(status: item.status, compact: true),
                    ] else
                      Tooltip(
                        message: item.status.label,
                        child: const Icon(
                          Icons.info_outline_rounded,
                          color: AppColors.muted,
                          size: 20,
                        ),
                      ),
                  ],
                );
              },
            ),
          ),
          const Divider(height: 1),
          Expanded(
            child: ListView.builder(
              controller: _scrollController,
              padding: const EdgeInsets.fromLTRB(16, 14, 16, 10),
              itemCount: item.messages.length +
                  (widget.controller.isBotTyping ? 1 : 0),
              itemBuilder: (BuildContext context, int index) {
                if (index == item.messages.length) {
                  return const _TypingBubble();
                }
                return ChatBubble(message: item.messages[index]);
              },
            ),
          ),
          _CustomerActionArea(
            controller: widget.controller,
            item: item,
          ),
          const Divider(height: 1),
          Padding(
            padding: const EdgeInsets.all(12),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: <Widget>[
                Expanded(
                  child: TextField(
                    controller: _inputController,
                    enabled: inputEnabled,
                    minLines: 1,
                    maxLines: 4,
                    textInputAction: TextInputAction.send,
                    onSubmitted: (String value) => _sendMessage(),
                    decoration: InputDecoration(
                      hintText: item.status == SupportCaseStatus.resolved
                          ? 'Atendimento concluído'
                          : 'Digite sua mensagem',
                      prefixIcon: const Icon(Icons.chat_outlined),
                    ),
                  ),
                ),
                const SizedBox(width: 9),
                SizedBox(
                  width: 50,
                  height: 50,
                  child: FilledButton(
                    onPressed: inputEnabled ? _sendMessage : null,
                    style: FilledButton.styleFrom(
                      padding: EdgeInsets.zero,
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(15),
                      ),
                    ),
                    child: const Icon(Icons.send_rounded),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _ChatToolbar extends StatelessWidget {
  const _ChatToolbar({required this.controller});

  final OrionController controller;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 11),
      child: LayoutBuilder(
        builder: (BuildContext context, BoxConstraints constraints) {
          final bool wide = constraints.maxWidth >= 670;
          final Widget selector = Wrap(
            spacing: 7,
            runSpacing: 7,
            children: UserChannel.values.map((UserChannel channel) {
              final bool selected = controller.currentChannel == channel;
              return ChoiceChip(
                selected: selected,
                label: Text(channel.label),
                avatar: Icon(
                  _channelIcon(channel),
                  size: 17,
                  color: selected ? AppColors.claroRed : AppColors.muted,
                ),
                onSelected: (bool value) {
                  if (value) {
                    controller.switchChannel(channel);
                  }
                },
              );
            }).toList(),
          );

          final Widget title = Row(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              const Icon(
                Icons.devices_other_rounded,
                size: 19,
                color: AppColors.claroRed,
              ),
              const SizedBox(width: 8),
              Flexible(
                child: Text(
                  'Continuar em outro canal',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
              ),
            ],
          );

          if (!wide) {
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                title,
                const SizedBox(height: 9),
                selector,
              ],
            );
          }

          return Row(
            children: <Widget>[
              Expanded(child: title),
              selector,
            ],
          );
        },
      ),
    );
  }

  IconData _channelIcon(UserChannel channel) {
    switch (channel) {
      case UserChannel.appClaro:
        return Icons.phone_iphone_rounded;
      case UserChannel.webPortal:
        return Icons.language_rounded;
      case UserChannel.whatsapp:
        return Icons.forum_outlined;
    }
  }
}

class _CustomerActionArea extends StatelessWidget {
  const _CustomerActionArea({
    required this.controller,
    required this.item,
  });

  final OrionController controller;
  final SupportCase item;

  @override
  Widget build(BuildContext context) {
    if (item.pendingAction == OrionController.restartAction) {
      return _ActionBox(
        icon: Icons.restart_alt_rounded,
        title: 'Ação sugerida',
        description: 'Autorize o reinício simulado do sinal.',
        accent: AppColors.info,
        actions: <Widget>[
          FilledButton.icon(
            onPressed: controller.confirmRestart,
            icon: const Icon(Icons.check_rounded),
            label: const Text('Sim, reiniciar'),
          ),
          OutlinedButton(
            onPressed: controller.declineRestart,
            child: const Text('Agora não'),
          ),
        ],
      );
    }

    if (item.pendingAction == OrionController.continueAction) {
      return _ActionBox(
        icon: Icons.sync_rounded,
        title: 'Ação pendente recuperada',
        description:
            'O contexto da sessão foi encontrado no novo canal.',
        accent: AppColors.purple,
        actions: <Widget>[
          FilledButton.icon(
            onPressed: controller.continueHere,
            icon: const Icon(Icons.play_arrow_rounded),
            label: const Text('Continuar aqui'),
          ),
          OutlinedButton(
            onPressed: controller.declineRestart,
            child: const Text('Cancelar ação'),
          ),
        ],
      );
    }

    if (item.status == SupportCaseStatus.waitingHuman) {
      return const _ActionBox(
        icon: Icons.hourglass_top_rounded,
        title: 'Transferência em andamento',
        description:
            'Um atendente verá o resumo e todo o histórico desta sessão.',
        accent: AppColors.warning,
        actions: <Widget>[],
      );
    }

    if (item.status == SupportCaseStatus.inProgress) {
      return _ActionBox(
        icon: Icons.support_agent_rounded,
        title: 'Atendente conectado',
        description:
            '${item.assignedAgent ?? 'Uma pessoa atendente'} assumiu esta conversa.',
        accent: AppColors.purple,
        actions: const <Widget>[],
      );
    }

    if (item.status == SupportCaseStatus.resolved) {
      return _ActionBox(
        icon: Icons.check_circle_outline_rounded,
        title: 'Atendimento concluído',
        description: 'O histórico e o contexto desta sessão foram salvos.',
        accent: AppColors.success,
        actions: <Widget>[
          OutlinedButton.icon(
            onPressed: controller.resetCustomerConversation,
            icon: const Icon(Icons.add_comment_outlined),
            label: const Text('Novo atendimento'),
          ),
        ],
      );
    }

    if (item.messages.length <= 1) {
      return Padding(
        padding: const EdgeInsets.fromLTRB(14, 4, 14, 12),
        child: Wrap(
          spacing: 8,
          runSpacing: 8,
          children: <Widget>[
            ActionChip(
              avatar: const Icon(Icons.wifi_rounded, size: 17),
              label: const Text('Minha internet está lenta'),
              onPressed: () => controller.sendCustomerMessage(
                'Minha internet está lenta.',
              ),
            ),
            ActionChip(
              avatar: const Icon(Icons.receipt_long_outlined, size: 17),
              label: const Text('Cobrança indevida'),
              onPressed: () => controller.sendCustomerMessage(
                'Quero contestar uma cobrança indevida na minha fatura.',
              ),
            ),
          ],
        ),
      );
    }

    return const SizedBox.shrink();
  }
}

class _ActionBox extends StatelessWidget {
  const _ActionBox({
    required this.icon,
    required this.title,
    required this.description,
    required this.accent,
    required this.actions,
  });

  final IconData icon;
  final String title;
  final String description;
  final Color accent;
  final List<Widget> actions;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.fromLTRB(12, 4, 12, 10),
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: accent.withValues(alpha: 0.07),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: accent.withValues(alpha: 0.25)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Icon(icon, color: accent, size: 20),
              const SizedBox(width: 9),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    Text(
                      title,
                      style: TextStyle(
                        color: accent,
                        fontWeight: FontWeight.w900,
                      ),
                    ),
                    const SizedBox(height: 3),
                    Text(
                      description,
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
            ],
          ),
          if (actions.isNotEmpty) ...<Widget>[
            const SizedBox(height: 12),
            Wrap(spacing: 8, runSpacing: 8, children: actions),
          ],
        ],
      ),
    );
  }
}

class _TypingBubble extends StatelessWidget {
  const _TypingBubble();

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 5),
        padding: const EdgeInsets.symmetric(horizontal: 15, vertical: 12),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(color: AppColors.border),
          borderRadius: const BorderRadius.only(
            topLeft: Radius.circular(18),
            topRight: Radius.circular(18),
            bottomLeft: Radius.circular(4),
            bottomRight: Radius.circular(18),
          ),
        ),
        child: const Row(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            SizedBox(
              width: 15,
              height: 15,
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
            SizedBox(width: 9),
            Text(
              'Orion está analisando...',
              style: TextStyle(color: AppColors.muted, fontSize: 12),
            ),
          ],
        ),
      ),
    );
  }
}

class _CustomerContextPanel extends StatelessWidget {
  const _CustomerContextPanel({
    required this.controller,
    required this.item,
  });

  final OrionController controller;
  final SupportCase item;

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      child: Column(
        children: <Widget>[
          SurfaceCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text('Contexto da sessão',
                    style: Theme.of(context).textTheme.titleLarge),
                const SizedBox(height: 14),
                _ContextRow(label: 'Protocolo', value: item.id),
                _ContextRow(
                  label: 'Canal atual',
                  value: controller.currentChannel.label,
                ),
                _ContextRow(label: 'Intenção', value: item.intent),
                const SizedBox(height: 8),
                ConfidenceBar(value: item.intentConfidence),
                const SizedBox(height: 16),
                Text(
                  'Resumo da IA',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 7),
                Text(item.summary, style: Theme.of(context).textTheme.bodySmall),
              ],
            ),
          ),
          const SizedBox(height: 12),
          SurfaceCard(
            color: const Color(0xFFF5F1FF),
            borderColor: const Color(0xFFDED4FA),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                const Icon(
                  Icons.cloud_done_outlined,
                  color: AppColors.purple,
                ),
                const SizedBox(height: 10),
                Text(
                  'Contexto persistente',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 5),
                Text(
                  'A intenção, o histórico e a ação pendente permanecem associados ao protocolo.',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
                const SizedBox(height: 13),
                OutlinedButton.icon(
                  onPressed: controller.currentChannel == UserChannel.webPortal
                      ? null
                      : () => controller.switchChannel(UserChannel.webPortal),
                  icon: const Icon(Icons.language_rounded),
                  label: const Text('Continuar no Web Portal'),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _ContextRow extends StatelessWidget {
  const _ContextRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          SizedBox(
            width: 84,
            child: Text(label, style: Theme.of(context).textTheme.bodySmall),
          ),
          Expanded(
            child: Text(
              value,
              textAlign: TextAlign.right,
              style: const TextStyle(
                color: AppColors.ink,
                fontSize: 12,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _CustomerSessionsTab extends StatelessWidget {
  const _CustomerSessionsTab({
    required this.controller,
    required this.onOpenChat,
  });

  final OrionController controller;
  final VoidCallback onOpenChat;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (BuildContext context, Widget? child) {
        final SupportCase item = controller.customerCase;
        return SingleChildScrollView(
          child: PageContainer(
            maxWidth: 900,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  'Suas sessões',
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
                const SizedBox(height: 8),
                Text(
                  'Consulte o status e retome o atendimento pelo protocolo.',
                  style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                        color: AppColors.muted,
                      ),
                ),
                const SizedBox(height: 24),
                _SessionCard(
                  id: item.id,
                  title: item.intent == 'NÃO_CLASSIFICADA'
                      ? 'Atendimento atual'
                      : _intentTitle(item.intent),
                  description: item.summary,
                  status: item.status,
                  channel: controller.currentChannel.label,
                  onTap: onOpenChat,
                ),
                const SizedBox(height: 12),
                const _SessionCard(
                  id: 'CX-2026-0094',
                  title: 'Suporte técnico',
                  description:
                      'Sinal reiniciado e conexão restabelecida pelo Web Portal.',
                  status: SupportCaseStatus.resolved,
                  channel: 'Web Portal',
                ),
                const SizedBox(height: 12),
                const _SessionCard(
                  id: 'CX-2026-0061',
                  title: 'Dúvida sobre fatura',
                  description:
                      'Atendimento concluído por especialista após análise do histórico.',
                  status: SupportCaseStatus.resolved,
                  channel: 'App Claro',
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  static String _intentTitle(String intent) {
    switch (intent) {
      case 'SUPORTE_TECNICO':
        return 'Suporte técnico';
      case 'CONTESTACAO_FATURA':
        return 'Contestação de fatura';
      default:
        return 'Atendimento atual';
    }
  }
}

class _SessionCard extends StatelessWidget {
  const _SessionCard({
    required this.id,
    required this.title,
    required this.description,
    required this.status,
    required this.channel,
    this.onTap,
  });

  final String id;
  final String title;
  final String description;
  final SupportCaseStatus status;
  final String channel;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return SurfaceCard(
      onTap: onTap,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Container(
            width: 46,
            height: 46,
            decoration: BoxDecoration(
              color: AppColors.canvas,
              borderRadius: BorderRadius.circular(14),
            ),
            child: const Icon(Icons.forum_outlined),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: <Widget>[
                    Text(title, style: Theme.of(context).textTheme.titleMedium),
                    StatusPill(status: status, compact: true),
                  ],
                ),
                const SizedBox(height: 5),
                Text(
                  '$id • $channel',
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        fontWeight: FontWeight.w700,
                      ),
                ),
                const SizedBox(height: 8),
                Text(
                  description,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ],
            ),
          ),
          if (onTap != null) ...<Widget>[
            const SizedBox(width: 8),
            const Icon(Icons.arrow_forward_rounded, color: AppColors.claroRed),
          ],
        ],
      ),
    );
  }
}
