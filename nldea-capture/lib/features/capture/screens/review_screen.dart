import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/providers.dart';

/// Post-capture review: one card per clip (thumbnail still when the device
/// allowed one, otherwise a placeholder), retake per clip, then finalize —
/// which writes manifest.json + checksums.json and locks the session.
class ReviewScreen extends ConsumerStatefulWidget {
  const ReviewScreen({super.key});

  @override
  ConsumerState<ReviewScreen> createState() => _ReviewScreenState();
}

class _ReviewScreenState extends ConsumerState<ReviewScreen> {
  bool _finalizing = false;

  Future<void> _finalize() async {
    setState(() => _finalizing = true);
    try {
      final manifest =
          await ref.read(activeSessionProvider.notifier).finalize();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(
          content: Text('Session ${manifest.sessionId.substring(0, 8)}… '
              'finalized (${manifest.clips.length} clips)')));
      context.go('/');
    } catch (e) {
      if (!mounted) return;
      setState(() => _finalizing = false);
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('Finalize failed: $e')));
    }
  }

  Future<void> _discard() async {
    await ref.read(activeSessionProvider.notifier).discard();
    if (mounted) context.go('/');
  }

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(activeSessionProvider);
    if (session == null) {
      return const Scaffold(body: Center(child: Text('No active session')));
    }
    return Scaffold(
      appBar: AppBar(title: const Text('Review session')),
      body: Column(
        children: [
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.all(16),
              itemCount: session.clips.length,
              itemBuilder: (context, i) {
                final clip = session.clips[i];
                final thumb = clip.thumbnailPath;
                return Card(
                  child: ListTile(
                    leading: SizedBox(
                      width: 56,
                      height: 56,
                      child: thumb != null && File(thumb).existsSync()
                          ? ClipRRect(
                              borderRadius: BorderRadius.circular(8),
                              child:
                                  Image.file(File(thumb), fit: BoxFit.cover),
                            )
                          : const Icon(Icons.videocam, size: 40),
                    ),
                    title: Text(clip.fileName),
                    subtitle: Text(
                        '${clip.challenge?.wire ?? 'neutral'} · '
                        '${clip.durationMs} ms · ${clip.fps} fps'),
                    trailing: TextButton.icon(
                      icon: const Icon(Icons.refresh),
                      label: const Text('Retake'),
                      onPressed: _finalizing
                          ? null
                          : () => context.go('/capture?retake=$i'),
                    ),
                  ),
                );
              },
            ),
          ),
          SafeArea(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Expanded(
                    child: OutlinedButton.icon(
                      icon: const Icon(Icons.delete_outline),
                      label: const Text('Discard'),
                      onPressed: _finalizing ? null : _discard,
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    flex: 2,
                    child: FilledButton.icon(
                      icon: const Icon(Icons.lock),
                      label: Text(_finalizing
                          ? 'Finalizing…'
                          : 'Finalize session (${session.clips.length} clips)'),
                      onPressed: _finalizing || session.clips.isEmpty
                          ? null
                          : _finalize,
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
