import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/models/manifest.dart';
import '../../../core/providers.dart';

/// Campaign device fleet (doc 10 quotas: actual Kenyan market). Free text is
/// allowed for replacements; the model string goes into the manifest as-is.
const List<String> deviceFleet = [
  'Tecno Spark 20',
  'Redmi 13C',
  'Samsung Galaxy A15',
  'Tecno Camon 20',
];

/// Session parameters: genuine vs attack, attack species, lighting station,
/// capture device. One session = one lighting station; the operator loops
/// back here per station.
class SessionSetupScreen extends ConsumerStatefulWidget {
  const SessionSetupScreen({super.key});

  @override
  ConsumerState<SessionSetupScreen> createState() =>
      _SessionSetupScreenState();
}

class _SessionSetupScreenState extends ConsumerState<SessionSetupScreen> {
  SessionType _type = SessionType.genuine;
  AttackType? _attackType;
  Lighting? _lighting;
  String _deviceModel = deviceFleet.first;
  int _attackClipCount = 4;

  bool get _valid =>
      _lighting != null &&
      (_type == SessionType.genuine || _attackType != null);

  void _start() {
    ref.read(activeSessionProvider.notifier).start(
          type: _type,
          attackType: _attackType,
          lighting: _lighting!,
          deviceModel: _deviceModel,
          attackClipCount: _attackClipCount,
        );
    // Genuine sessions offer the optional Monk self-report first; attack
    // sessions capture presentation media, so skin tone is not asked.
    context.go(_type == SessionType.genuine ? '/skin-tone' : '/capture');
  }

  @override
  Widget build(BuildContext context) {
    final consent = ref.watch(activeConsentProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Session setup')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Card(
            child: ListTile(
              leading: const Icon(Icons.badge),
              title: Text('Subject ${consent?.subjectId ?? '—'}'),
              subtitle: const Text(
                  'Write this ID on the participant card — it is the only '
                  'key to withdrawal'),
            ),
          ),
          const SizedBox(height: 16),
          SegmentedButton<SessionType>(
            segments: const [
              ButtonSegment(
                  value: SessionType.genuine,
                  label: Text('Genuine'),
                  icon: Icon(Icons.face)),
              ButtonSegment(
                  value: SessionType.attack,
                  label: Text('Attack'),
                  icon: Icon(Icons.warning_amber)),
            ],
            selected: {_type},
            onSelectionChanged: (s) => setState(() {
              _type = s.first;
              if (_type == SessionType.genuine) _attackType = null;
            }),
          ),
          if (_type == SessionType.attack) ...[
            const SizedBox(height: 16),
            Text('Attack species',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final t in AttackType.values)
                  ChoiceChip(
                    label: Text(t.label),
                    selected: _attackType == t,
                    // mask_3d is in the schema but deferred to the L2 phase.
                    onSelected: t == AttackType.mask3d
                        ? null
                        : (v) =>
                            setState(() => _attackType = v ? t : null),
                  ),
              ],
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                const Expanded(child: Text('Clips to capture')),
                IconButton(
                  icon: const Icon(Icons.remove_circle_outline),
                  onPressed: _attackClipCount > 1
                      ? () => setState(() => _attackClipCount--)
                      : null,
                ),
                Text('$_attackClipCount'),
                IconButton(
                  icon: const Icon(Icons.add_circle_outline),
                  onPressed: _attackClipCount < 12
                      ? () => setState(() => _attackClipCount++)
                      : null,
                ),
              ],
            ),
          ],
          const SizedBox(height: 16),
          Text('Lighting station',
              style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          RadioGroup<Lighting>(
            groupValue: _lighting,
            onChanged: (v) => setState(() => _lighting = v),
            child: Column(
              children: [
                for (final l in Lighting.values)
                  RadioListTile<Lighting>(value: l, title: Text(l.label)),
              ],
            ),
          ),
          const SizedBox(height: 16),
          DropdownMenu<String>(
            initialSelection: _deviceModel,
            label: const Text('Capture device'),
            expandedInsets: EdgeInsets.zero,
            dropdownMenuEntries: [
              for (final d in deviceFleet)
                DropdownMenuEntry(value: d, label: d),
            ],
            onSelected: (v) =>
                setState(() => _deviceModel = v ?? _deviceModel),
          ),
          const SizedBox(height: 24),
          FilledButton.icon(
            icon: const Icon(Icons.videocam),
            label: const Text('Start capture'),
            onPressed: _valid ? _start : null,
          ),
        ],
      ),
    );
  }
}
