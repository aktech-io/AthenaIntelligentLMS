import 'dart:async';
import 'dart:io';
import 'dart:math';

import 'package:camera/camera.dart';
import 'package:flutter/material.dart';

/// Result of one recorded clip.
class RecordedClip {
  const RecordedClip({
    required this.filePath,
    required this.durationMs,
    required this.fps,
    this.thumbnailPath,
  });

  final String filePath;
  final int durationMs;

  /// Target fps the recorder was configured for (recorded in the manifest).
  final int fps;

  /// Optional still captured right after the clip, for the review screen.
  final String? thumbnailPath;
}

/// Camera seam. The real implementation wraps the `camera` plugin; tests and
/// camera-less dev machines use [FakeCaptureCamera] via a provider override,
/// so `flutter test` never needs a device.
abstract class CaptureCamera {
  /// Target capture rate recorded into manifests.
  int get targetFps;

  Future<void> initialize();

  /// Live preview widget; only valid between [initialize] and [dispose].
  Widget buildPreview(BuildContext context);

  /// Records ~[duration] of video into [outputPath] (an .mp4 path inside the
  /// session directory) and captures a review thumbnail next to it.
  Future<RecordedClip> recordClip({
    required Duration duration,
    required String outputPath,
  });

  Future<void> dispose();
}

/// Real front-camera implementation on the `camera` plugin. Mirrors the
/// NemoWallet selfie-screen setup (front lens, no audio, medium preset).
class PluginCaptureCamera implements CaptureCamera {
  CameraController? _controller;

  @override
  int get targetFps => 30;

  @override
  Future<void> initialize() async {
    if (_controller != null) return;
    final cameras = await availableCameras();
    if (cameras.isEmpty) {
      throw CameraException('noCamera', 'No cameras available');
    }
    final front = cameras.firstWhere(
      (c) => c.lensDirection == CameraLensDirection.front,
      orElse: () => cameras.first,
    );
    final controller = CameraController(
      front,
      ResolutionPreset.medium,
      enableAudio: false,
    );
    await controller.initialize();
    _controller = controller;
  }

  @override
  Widget buildPreview(BuildContext context) {
    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) {
      return const ColoredBox(
        color: Colors.black,
        child: Center(child: CircularProgressIndicator()),
      );
    }
    return CameraPreview(controller);
  }

  @override
  Future<RecordedClip> recordClip({
    required Duration duration,
    required String outputPath,
  }) async {
    final controller = _controller;
    if (controller == null) throw StateError('Camera not initialized');

    final started = DateTime.now();
    await controller.startVideoRecording();
    await Future<void>.delayed(duration);
    final recording = await controller.stopVideoRecording();
    final actualMs = DateTime.now().difference(started).inMilliseconds;

    // Move the plugin's temp file into the session directory.
    final saved = await File(recording.path).copy(outputPath);
    await File(recording.path).delete();

    // Best-effort review thumbnail (a still right after the clip).
    String? thumbPath;
    try {
      final still = await controller.takePicture();
      thumbPath = outputPath.replaceFirst(RegExp(r'\.mp4$'), '_thumb.jpg');
      await File(still.path).copy(thumbPath);
      await File(still.path).delete();
    } catch (_) {
      thumbPath = null; // some devices dislike takePicture around recording
    }

    return RecordedClip(
      filePath: saved.path,
      durationMs: actualMs,
      fps: targetFps,
      thumbnailPath: thumbPath,
    );
  }

  @override
  Future<void> dispose() async {
    await _controller?.dispose();
    _controller = null;
  }
}

/// Deterministic fake for widget tests and camera-less development: writes
/// small placeholder files instead of real media.
class FakeCaptureCamera implements CaptureCamera {
  FakeCaptureCamera({this.recordDelay = Duration.zero});

  final Duration recordDelay;
  final Random _rng = Random(42);
  bool initialized = false;
  int recordedCount = 0;

  @override
  int get targetFps => 30;

  @override
  Future<void> initialize() async {
    initialized = true;
  }

  @override
  Widget buildPreview(BuildContext context) => Container(
        color: Colors.black,
        alignment: Alignment.center,
        child: const Text('FAKE CAMERA PREVIEW',
            style: TextStyle(color: Colors.white54)),
      );

  @override
  Future<RecordedClip> recordClip({
    required Duration duration,
    required String outputPath,
  }) async {
    if (!initialized) throw StateError('FakeCaptureCamera not initialized');
    if (recordDelay > Duration.zero) {
      await Future<void>.delayed(recordDelay);
    }
    recordedCount++;
    final payload =
        List<int>.generate(256, (_) => _rng.nextInt(256), growable: false);
    await File(outputPath).writeAsBytes(payload);
    return RecordedClip(
      filePath: outputPath,
      durationMs: duration.inMilliseconds,
      fps: targetFps,
      thumbnailPath: null,
    );
  }

  @override
  Future<void> dispose() async {
    initialized = false;
  }
}
