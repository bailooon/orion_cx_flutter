import 'package:flutter/material.dart';

import '../core/app_theme.dart';
import '../core/orion_controller.dart';

/// Satisfaction survey shown in the chat once an attendance is resolved.
///
/// One tap gives the score; the comment is optional, so answering costs the
/// customer a few seconds. The answer feeds the satisfaction dashboard, split
/// by who solved the case (AI or a person), intent and channel.
class SatisfactionSurveyCard extends StatefulWidget {
  const SatisfactionSurveyCard({required this.controller, super.key});

  final OrionController controller;

  @override
  State<SatisfactionSurveyCard> createState() => _SatisfactionSurveyCardState();
}

class _SatisfactionSurveyCardState extends State<SatisfactionSurveyCard> {
  final TextEditingController _comment = TextEditingController();
  int _score = 0;

  static const List<String> _labels = <String>[
    'Muito insatisfeito',
    'Insatisfeito',
    'Neutro',
    'Satisfeito',
    'Muito satisfeito',
  ];

  @override
  void dispose() {
    _comment.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (_score == 0) {
      return;
    }
    await widget.controller.submitSurvey(_score, _comment.text);
  }

  @override
  Widget build(BuildContext context) {
    final OrionController controller = widget.controller;
    final TextTheme text = Theme.of(context).textTheme;

    return Container(
      margin: const EdgeInsets.fromLTRB(14, 4, 14, 12),
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: const Color(0xFFF2FAF6),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: const Color(0xFFBFE6D2)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Row(
            children: <Widget>[
              const Icon(Icons.sentiment_satisfied_alt_rounded,
                  color: AppColors.success, size: 20),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  'Atendimento concluído. Como foi a sua experiência?',
                  style: text.titleSmall?.copyWith(fontWeight: FontWeight.w800),
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 2,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: <Widget>[
              for (int value = 1; value <= 5; value++)
                IconButton(
                  tooltip: _labels[value - 1],
                  visualDensity: VisualDensity.compact,
                  onPressed: controller.surveySubmitting
                      ? null
                      : () => setState(() => _score = value),
                  icon: Icon(
                    value <= _score
                        ? Icons.star_rounded
                        : Icons.star_outline_rounded,
                    color: value <= _score
                        ? const Color(0xFFF5A300)
                        : AppColors.muted,
                    size: 30,
                  ),
                ),
              if (_score > 0)
                Padding(
                  padding: const EdgeInsets.only(left: 6),
                  child: Text(
                    _labels[_score - 1],
                    style: text.bodySmall?.copyWith(fontWeight: FontWeight.w700),
                  ),
                ),
            ],
          ),
          if (_score > 0) ...<Widget>[
            const SizedBox(height: 8),
            TextField(
              controller: _comment,
              maxLength: 500,
              minLines: 1,
              maxLines: 3,
              decoration: const InputDecoration(
                hintText: 'Quer contar mais? (opcional)',
                isDense: true,
              ),
            ),
          ],
          if (controller.surveyError != null) ...<Widget>[
            const SizedBox(height: 6),
            Text(
              controller.surveyError!,
              style: text.bodySmall?.copyWith(color: AppColors.claroRed),
            ),
          ],
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: <Widget>[
              FilledButton.icon(
                onPressed:
                    _score == 0 || controller.surveySubmitting ? null : _submit,
                icon: controller.surveySubmitting
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                            strokeWidth: 2, color: Colors.white),
                      )
                    : const Icon(Icons.send_rounded, size: 18),
                label: const Text('Enviar avaliação'),
              ),
              TextButton.icon(
                onPressed: controller.resetCustomerConversation,
                icon: const Icon(Icons.add_comment_outlined, size: 18),
                label: const Text('Novo atendimento'),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
