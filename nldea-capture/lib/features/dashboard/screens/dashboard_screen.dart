import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/models/manifest.dart';
import '../../../core/providers.dart';

/// Campaign progress vs the doc-10 targets: subjects (gate G2), genuine
/// clips, attack clips per species, lighting mix and the Monk 7-10 quota.
class DashboardScreen extends ConsumerWidget {
  const DashboardScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final targets = ref.watch(campaignTargetsProvider);
    final statsAsync = ref.watch(campaignStatsProvider);

    return Scaffold(
      appBar: AppBar(title: const Text('Campaign dashboard')),
      body: statsAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed to load stats: $e')),
        data: (stats) => ListView(
          padding: const EdgeInsets.all(16),
          children: [
            _ProgressTile(
              label: 'Subjects (G3 target ${targets.totalSubjects})',
              value: stats.subjectCount,
              target: targets.totalSubjects,
            ),
            _ProgressTile(
              label: 'Gate G2 — ${targets.g2Subjects} subjects '
                  '(first fine-tune)',
              value: stats.subjectCount,
              target: targets.g2Subjects,
              highlight: stats.g2Reached(targets),
            ),
            const Divider(height: 32),
            _ProgressTile(
              label: 'Genuine clips',
              value: stats.genuineClipCount,
              target: targets.genuineClips,
            ),
            const SizedBox(height: 16),
            Text('Attack clips by species',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            for (final entry in targets.attackClips.entries)
              _ProgressTile(
                label: entry.key.label,
                value: stats.attackClipCounts[entry.key] ?? 0,
                target: entry.value,
              ),
            const Divider(height: 32),
            Text('Clips by lighting station',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            for (final l in Lighting.values)
              ListTile(
                dense: true,
                title: Text(l.label),
                trailing: Text('${stats.sessionsByLighting[l]}'),
              ),
            const Divider(height: 32),
            Text('Skin tone (Monk) — quota: >=60% of clips in 7–10',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            ListTile(
              dense: true,
              title: const Text('Share of tone-reported clips in Monk 7–10'),
              trailing: Text(stats.darkToneShare == null
                  ? '—'
                  : '${(stats.darkToneShare! * 100).toStringAsFixed(0)}%'),
              subtitle: stats.darkToneShare != null &&
                      stats.darkToneShare! < targets.minDarkToneShare
                  ? const Text('BELOW QUOTA — prioritize Monk 7–10 subjects')
                  : null,
            ),
            for (final t in MonkTone.values)
              if ((stats.skinToneClipCounts[t] ?? 0) > 0)
                ListTile(
                  dense: true,
                  leading: CircleAvatar(
                      backgroundColor: Color(t.argb), radius: 10),
                  title: Text(t.wire),
                  trailing: Text('${stats.skinToneClipCounts[t]}'),
                ),
          ],
        ),
      ),
    );
  }
}

class _ProgressTile extends StatelessWidget {
  const _ProgressTile({
    required this.label,
    required this.value,
    required this.target,
    this.highlight = false,
  });

  final String label;
  final int value;
  final int target;
  final bool highlight;

  @override
  Widget build(BuildContext context) {
    final fraction = target == 0 ? 0.0 : (value / target).clamp(0.0, 1.0);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(child: Text(label)),
              Text('$value / $target',
                  style: TextStyle(
                      fontWeight:
                          highlight ? FontWeight.bold : FontWeight.normal)),
              if (highlight)
                const Padding(
                  padding: EdgeInsets.only(left: 4),
                  child: Icon(Icons.check_circle,
                      size: 16, color: Colors.green),
                ),
            ],
          ),
          const SizedBox(height: 4),
          LinearProgressIndicator(value: fraction),
        ],
      ),
    );
  }
}
