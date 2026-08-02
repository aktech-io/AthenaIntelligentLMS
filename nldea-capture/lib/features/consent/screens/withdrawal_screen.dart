import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/models/consent_record.dart';
import '../../../core/models/manifest.dart';
import '../../../core/providers.dart';

/// Withdrawal-by-subject-ID (DPIA commitment): the operator enters the
/// pseudonym from the participant card, marks consent withdrawn, and
/// deletes that subject's session directories.
class WithdrawalScreen extends ConsumerStatefulWidget {
  const WithdrawalScreen({super.key});

  @override
  ConsumerState<WithdrawalScreen> createState() => _WithdrawalScreenState();
}

class _WithdrawalScreenState extends ConsumerState<WithdrawalScreen> {
  final _controller = TextEditingController();
  List<ConsentRecord> _records = [];
  List<SessionManifest> _sessions = [];
  bool _searched = false;
  bool _busy = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  String get _subjectId => _controller.text.trim().toUpperCase();

  Future<void> _lookup() async {
    setState(() => _busy = true);
    final records =
        await ref.read(consentStoreProvider).forSubject(_subjectId);
    final sessions =
        await ref.read(sessionStoreProvider).sessionsForSubject(_subjectId);
    setState(() {
      _records = records;
      _sessions = sessions;
      _searched = true;
      _busy = false;
    });
  }

  Future<void> _withdraw() async {
    setState(() => _busy = true);
    await ref.read(consentStoreProvider).markWithdrawn(_subjectId);
    await _lookup();
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(
          content: Text('Consent for $_subjectId marked withdrawn. '
              'Now delete the sessions below.')));
    }
  }

  Future<void> _deleteSession(String sessionId) async {
    setState(() => _busy = true);
    await ref.read(sessionStoreProvider).deleteSession(sessionId);
    ref.read(sessionsChangedProvider.notifier).state++;
    await _lookup();
  }

  Future<void> _deleteAll() async {
    setState(() => _busy = true);
    for (final s in _sessions) {
      await ref.read(sessionStoreProvider).deleteSession(s.sessionId);
    }
    ref.read(sessionsChangedProvider.notifier).state++;
    await _lookup();
  }

  @override
  Widget build(BuildContext context) {
    final withdrawn = _records.isNotEmpty && _records.every(
        (r) => r.isWithdrawn);
    return Scaffold(
      appBar: AppBar(title: const Text('Withdraw consent')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          TextField(
            controller: _controller,
            textCapitalization: TextCapitalization.characters,
            decoration: InputDecoration(
              labelText: 'Subject ID (participant card)',
              hintText: 'NLD-XXXXXXXX',
              border: const OutlineInputBorder(),
              suffixIcon: IconButton(
                icon: const Icon(Icons.search),
                onPressed: _busy ? null : _lookup,
              ),
            ),
            onSubmitted: (_) => _lookup(),
          ),
          const SizedBox(height: 16),
          if (_searched && _records.isEmpty)
            const Card(
              child: ListTile(
                leading: Icon(Icons.help_outline),
                title: Text('No consent record for that subject ID'),
              ),
            ),
          if (_records.isNotEmpty) ...[
            Card(
              child: Column(
                children: [
                  for (final r in _records)
                    ListTile(
                      leading: Icon(r.isWithdrawn
                          ? Icons.block
                          : Icons.check_circle_outline),
                      title: Text('Consent ${r.consentId.substring(0, 10)}…'),
                      subtitle: Text(r.isWithdrawn
                          ? 'WITHDRAWN ${r.withdrawnAt}'
                          : 'Agreed ${r.agreedAt} '
                              '(${r.consentTextVersion})'),
                    ),
                ],
              ),
            ),
            const SizedBox(height: 8),
            if (!withdrawn)
              FilledButton.icon(
                icon: const Icon(Icons.person_off),
                label: const Text('Mark consent WITHDRAWN'),
                onPressed: _busy ? null : _withdraw,
              ),
            const SizedBox(height: 16),
            Text('Sessions (${_sessions.length})',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            if (_sessions.isEmpty)
              const Text('No stored sessions for this subject.'),
            for (final s in _sessions)
              Card(
                child: ListTile(
                  leading: Icon(s.type == SessionType.genuine
                      ? Icons.face
                      : Icons.warning_amber),
                  title: Text('${s.type.wire}'
                      '${s.attackType == null ? '' : ' · ${s.attackType!.wire}'}'
                      ' · ${s.lighting.wire}'),
                  subtitle: Text(
                      '${s.clips.length} clips · ${s.sessionId.substring(0, 8)}…'),
                  trailing: IconButton(
                    icon: const Icon(Icons.delete_forever),
                    onPressed:
                        _busy ? null : () => _deleteSession(s.sessionId),
                  ),
                ),
              ),
            if (_sessions.isNotEmpty) ...[
              const SizedBox(height: 8),
              OutlinedButton.icon(
                icon: const Icon(Icons.delete_sweep),
                label: const Text('Delete ALL sessions for this subject'),
                onPressed: _busy ? null : _deleteAll,
              ),
            ],
          ],
        ],
      ),
    );
  }
}
