import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/models/consent_record.dart';
import '../../../core/providers.dart';
import '../../../core/services/ids.dart';

/// The consent gate. The subject reads the consent copy (PLACEHOLDER,
/// pending DPIA counsel review), ticks the confirmation and taps "I agree".
/// Only then does the app mint a pseudonymous subjectId + consentId, store
/// the consent record (separately from media) and unlock the capture routes.
class ConsentScreen extends ConsumerStatefulWidget {
  const ConsentScreen({super.key});

  @override
  ConsumerState<ConsentScreen> createState() => _ConsentScreenState();
}

class _ConsentScreenState extends ConsumerState<ConsentScreen> {
  late final TextEditingController _operatorController;
  bool _confirmed = false;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _operatorController =
        TextEditingController(text: ref.read(operatorIdProvider));
  }

  @override
  void dispose() {
    _operatorController.dispose();
    super.dispose();
  }

  Future<void> _agree() async {
    final operatorId = _operatorController.text.trim();
    if (operatorId.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Enter your operator ID first')));
      return;
    }
    setState(() => _saving = true);
    ref.read(operatorIdProvider.notifier).set(operatorId);

    final consentStore = ref.read(consentStoreProvider);
    var subjectId = generateSubjectId();
    // Defensive: regenerate on the (astronomically unlikely) collision.
    while (await consentStore.subjectIdExists(subjectId)) {
      subjectId = generateSubjectId();
    }
    final record = ConsentRecord(
      consentId: generateConsentId(),
      subjectId: subjectId,
      consentTextVersion: consentTextVersion,
      operatorId: operatorId,
      agreedAt: DateTime.now().toUtc(),
    );
    await consentStore.save(record);
    ref.read(activeConsentProvider.notifier).state = record;
    if (mounted) context.go('/setup');
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Participant consent')),
      body: Column(
        children: [
          Container(
            width: double.infinity,
            color: Colors.amber.shade900,
            padding: const EdgeInsets.all(8),
            child: const Text(
              'PLACEHOLDER CONSENT COPY — pending DPIA counsel review. '
              'No field capture before gate G1 (DPIA filed).',
              style: TextStyle(fontWeight: FontWeight.bold),
            ),
          ),
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: Text(consentText),
            ),
          ),
          SafeArea(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  TextField(
                    controller: _operatorController,
                    decoration: const InputDecoration(
                      labelText: 'Operator ID',
                      helperText: 'Field agent code — recorded with consent, '
                          'never with media',
                      border: OutlineInputBorder(),
                    ),
                  ),
                  CheckboxListTile(
                    value: _confirmed,
                    onChanged: (v) => setState(() => _confirmed = v ?? false),
                    title: const Text(
                        'Subject has read (or been read) the text above, is '
                        '18 or older, and agrees to participate'),
                    controlAffinity: ListTileControlAffinity.leading,
                  ),
                  Row(
                    children: [
                      Expanded(
                        child: OutlinedButton(
                          onPressed:
                              _saving ? null : () => context.go('/'),
                          child: const Text('Decline'),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: FilledButton(
                          onPressed:
                              _confirmed && !_saving ? _agree : null,
                          child: const Text('I agree'),
                        ),
                      ),
                    ],
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
