import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/models/manifest.dart';
import '../../../core/providers.dart';

/// Finalized-session listing with multi-select zip export via the share
/// sheet (v1 hand-off; the upload service is a later phase — see
/// StubUploadService).
class SessionsScreen extends ConsumerStatefulWidget {
  const SessionsScreen({super.key});

  @override
  ConsumerState<SessionsScreen> createState() => _SessionsScreenState();
}

class _SessionsScreenState extends ConsumerState<SessionsScreen> {
  final Set<String> _selected = {};
  bool _exporting = false;

  Future<void> _export(List<SessionManifest> manifests) async {
    final ids = _selected.isEmpty
        ? manifests.map((m) => m.sessionId).toList()
        : _selected.toList();
    if (ids.isEmpty) return;
    setState(() => _exporting = true);
    try {
      final export = ref.read(exportServiceProvider);
      final zip = await export.zipSessions(ids);
      await export.shareZip(zip);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
            content: Text('Exported ${ids.length} session(s) to '
                '${zip.path}')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Export failed: $e')));
      }
    } finally {
      if (mounted) setState(() => _exporting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final manifestsAsync = ref.watch(allManifestsProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Sessions & export')),
      body: manifestsAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed to list sessions: $e')),
        data: (manifests) {
          if (manifests.isEmpty) {
            return const Center(child: Text('No finalized sessions yet.'));
          }
          return Column(
            children: [
              Expanded(
                child: ListView.builder(
                  itemCount: manifests.length,
                  itemBuilder: (context, i) {
                    final m = manifests[i];
                    return CheckboxListTile(
                      value: _selected.contains(m.sessionId),
                      onChanged: (v) => setState(() {
                        if (v ?? false) {
                          _selected.add(m.sessionId);
                        } else {
                          _selected.remove(m.sessionId);
                        }
                      }),
                      title: Text('${m.subjectId} · ${m.type.wire}'
                          '${m.attackType == null ? '' : ' · ${m.attackType!.wire}'}'),
                      subtitle: Text('${m.lighting.wire} · '
                          '${m.clips.length} clips · '
                          '${m.capturedAt.toLocal()}'),
                    );
                  },
                ),
              ),
              SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: FilledButton.icon(
                    icon: const Icon(Icons.ios_share),
                    label: Text(_exporting
                        ? 'Exporting…'
                        : _selected.isEmpty
                            ? 'Export ALL (${manifests.length}) as zip'
                            : 'Export ${_selected.length} selected as zip'),
                    onPressed:
                        _exporting ? null : () => _export(manifests),
                  ),
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}
