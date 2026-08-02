import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/providers.dart';

/// Operator landing screen. "New subject session" leads into the
/// consent-gated capture flow; everything else is campaign management.
class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final stats = ref.watch(campaignStatsProvider);
    final consent = ref.watch(activeConsentProvider);

    return Scaffold(
      appBar: AppBar(title: const Text('NLD-EA Capture')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Campaign',
                      style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 8),
                  stats.when(
                    data: (s) => Text(
                        '${s.subjectCount} subjects · '
                        '${s.genuineClipCount} genuine · '
                        '${s.totalAttackClipCount} attack clips'),
                    loading: () => const Text('Loading…'),
                    error: (e, _) => Text('Stats unavailable: $e'),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
          FilledButton.icon(
            icon: const Icon(Icons.person_add),
            label: const Text('New subject session'),
            onPressed: () {
              // Fresh subject: drop any prior consent so the gate re-runs.
              ref.read(activeConsentProvider.notifier).state = null;
              context.go('/consent');
            },
          ),
          if (consent != null) ...[
            const SizedBox(height: 8),
            OutlinedButton.icon(
              icon: const Icon(Icons.replay),
              label: Text('Continue subject ${consent.subjectId} '
                  '(next station)'),
              onPressed: () => context.go('/setup'),
            ),
          ],
          const SizedBox(height: 8),
          OutlinedButton.icon(
            icon: const Icon(Icons.insights),
            label: const Text('Campaign dashboard'),
            onPressed: () => context.go('/dashboard'),
          ),
          const SizedBox(height: 8),
          OutlinedButton.icon(
            icon: const Icon(Icons.folder_zip),
            label: const Text('Sessions & export'),
            onPressed: () => context.go('/sessions'),
          ),
          const SizedBox(height: 8),
          OutlinedButton.icon(
            icon: const Icon(Icons.person_off),
            label: const Text('Withdraw consent / delete subject'),
            onPressed: () => context.go('/withdraw'),
          ),
        ],
      ),
    );
  }
}
