import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/models/manifest.dart';
import '../../../core/providers.dart';

/// Scripted capture. Genuine sessions run the doc-10 challenge script
/// (neutral -> blink -> turn left -> turn right -> smile); attack sessions
/// record N neutral clips of the presentation. Per clip: prompt, 3-2-1
/// countdown, ~3 s recording.
class GuidedCaptureScreen extends ConsumerStatefulWidget {
  const GuidedCaptureScreen({super.key});

  @override
  ConsumerState<GuidedCaptureScreen> createState() =>
      _GuidedCaptureScreenState();
}

enum _Phase { initializing, ready, countdown, recording, error }

class _GuidedCaptureScreenState extends ConsumerState<GuidedCaptureScreen> {
  _Phase _phase = _Phase.initializing;
  int _countdown = 0;
  String? _error;

  /// Retake target when launched from the review screen (`?retake=i`),
  /// null while walking the plan forward.
  int? _retakeIndex;

  @override
  void initState() {
    super.initState();
    _initCamera();
  }

  Future<void> _initCamera() async {
    try {
      await ref.read(captureCameraProvider).initialize();
      if (mounted) setState(() => _phase = _Phase.ready);
    } catch (e) {
      if (mounted) {
        setState(() {
          _phase = _Phase.error;
          _error = 'Camera unavailable: $e';
        });
      }
    }
  }

  @override
  void dispose() {
    ref.read(captureCameraProvider).dispose();
    super.dispose();
  }

  String _promptFor(Challenge? challenge) =>
      challenge?.label ?? 'Hold still, look at the camera';

  Future<void> _captureNext() async {
    final index =
        _retakeIndex ?? ref.read(activeSessionProvider)!.clips.length;
    setState(() {
      _phase = _Phase.countdown;
      _countdown = 3;
    });
    for (var i = 3; i > 0; i--) {
      if (!mounted) return;
      setState(() => _countdown = i);
      await Future<void>.delayed(const Duration(seconds: 1));
    }
    if (!mounted) return;
    setState(() => _phase = _Phase.recording);
    try {
      await ref
          .read(activeSessionProvider.notifier)
          .captureClip(retakeIndex: _retakeIndex);
    } catch (e) {
      if (mounted) {
        setState(() {
          _phase = _Phase.error;
          _error = 'Recording failed on clip ${index + 1}: $e';
        });
      }
      return;
    }
    if (!mounted) return;
    final session = ref.read(activeSessionProvider)!;
    if (_retakeIndex != null || session.isComplete) {
      context.go('/review');
    } else {
      setState(() => _phase = _Phase.ready);
    }
  }

  Future<void> _abort() async {
    await ref.read(activeSessionProvider.notifier).discard();
    if (mounted) context.go('/');
  }

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(activeSessionProvider);
    if (session == null) {
      // Session was discarded/finalized elsewhere.
      return const Scaffold(body: Center(child: Text('No active session')));
    }
    final uri = GoRouterState.of(context).uri;
    _retakeIndex = uri.queryParameters['retake'] != null
        ? int.tryParse(uri.queryParameters['retake']!)
        : null;

    final index = _retakeIndex ?? session.clips.length;
    final total = session.plan.length;
    final challenge =
        index < session.plan.length ? session.plan[index] : null;
    final camera = ref.read(captureCameraProvider);

    return Scaffold(
      appBar: AppBar(
        title: Text(_retakeIndex != null
            ? 'Retake clip ${index + 1}'
            : 'Clip ${(index + 1).clamp(1, total)} of $total'),
        actions: [
          if (_retakeIndex == null)
            IconButton(
              tooltip: 'Abort session',
              icon: const Icon(Icons.close),
              onPressed: _phase == _Phase.recording ? null : _abort,
            ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: Stack(
              fit: StackFit.expand,
              children: [
                if (_phase != _Phase.initializing && _phase != _Phase.error)
                  camera.buildPreview(context),
                if (_phase == _Phase.initializing)
                  const Center(child: CircularProgressIndicator()),
                if (_phase == _Phase.error)
                  Center(
                    child: Padding(
                      padding: const EdgeInsets.all(24),
                      child: Text(_error ?? 'Camera error',
                          textAlign: TextAlign.center),
                    ),
                  ),
                if (_phase == _Phase.countdown)
                  Center(
                    child: Text('$_countdown',
                        style: Theme.of(context)
                            .textTheme
                            .displayLarge
                            ?.copyWith(
                                color: Colors.white,
                                fontWeight: FontWeight.bold)),
                  ),
                if (_phase == _Phase.recording)
                  const Align(
                    alignment: Alignment.topCenter,
                    child: Padding(
                      padding: EdgeInsets.all(16),
                      child: Chip(
                        avatar: Icon(Icons.fiber_manual_record,
                            color: Colors.red),
                        label: Text('RECORDING'),
                      ),
                    ),
                  ),
              ],
            ),
          ),
          SafeArea(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    session.type == SessionType.attack
                        ? 'Present the ${session.attackType?.label ?? 'attack'}'
                            ' to the camera'
                        : _promptFor(challenge),
                    style: Theme.of(context).textTheme.titleLarge,
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 12),
                  FilledButton.icon(
                    icon: const Icon(Icons.fiber_manual_record),
                    label: Text(_phase == _Phase.ready
                        ? 'Record 3 s clip'
                        : 'Working…'),
                    onPressed: _phase == _Phase.ready ? _captureNext : null,
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
