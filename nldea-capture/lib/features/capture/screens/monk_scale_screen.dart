import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/models/manifest.dart';
import '../../../core/providers.dart';

/// Optional Monk skin-tone self-report (quota axis: >=60% of clips in
/// Monk 7-10). The subject picks the swatch closest to their skin tone —
/// self-report, never operator-assigned. Skippable.
class MonkScaleScreen extends ConsumerStatefulWidget {
  const MonkScaleScreen({super.key});

  @override
  ConsumerState<MonkScaleScreen> createState() => _MonkScaleScreenState();
}

class _MonkScaleScreenState extends ConsumerState<MonkScaleScreen> {
  MonkTone? _selected;

  void _continue({required bool skip}) {
    final notifier = ref.read(activeSessionProvider.notifier);
    final session = ref.read(activeSessionProvider);
    if (session != null) {
      notifier.update(skip
          ? session.copyWith(clearSkinTone: true)
          : session.copyWith(skinTone: _selected));
    }
    context.go('/capture');
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Skin tone (optional)')),
      body: Column(
        children: [
          const Padding(
            padding: EdgeInsets.all(16),
            child: Text(
                'Ask the subject to pick the swatch closest to their skin '
                'tone (Monk scale). This is a self-report and entirely '
                'optional.'),
          ),
          Expanded(
            child: GridView.count(
              crossAxisCount: 5,
              padding: const EdgeInsets.all(16),
              mainAxisSpacing: 12,
              crossAxisSpacing: 12,
              children: [
                for (final tone in MonkTone.values)
                  InkWell(
                    onTap: () => setState(() => _selected = tone),
                    child: Container(
                      decoration: BoxDecoration(
                        color: Color(tone.argb),
                        borderRadius: BorderRadius.circular(12),
                        border: Border.all(
                          width: 3,
                          color: _selected == tone
                              ? Theme.of(context).colorScheme.primary
                              : Colors.transparent,
                        ),
                      ),
                      alignment: Alignment.bottomCenter,
                      padding: const EdgeInsets.all(4),
                      child: Text(
                        '${tone.index + 1}',
                        style: TextStyle(
                          fontWeight: FontWeight.bold,
                          color: tone.index < 5
                              ? Colors.black87
                              : Colors.white70,
                        ),
                      ),
                    ),
                  ),
              ],
            ),
          ),
          SafeArea(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Expanded(
                    child: OutlinedButton(
                      onPressed: () => _continue(skip: true),
                      child: const Text('Skip'),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: FilledButton(
                      onPressed:
                          _selected == null ? null : () => _continue(skip: false),
                      child: const Text('Continue'),
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
}
