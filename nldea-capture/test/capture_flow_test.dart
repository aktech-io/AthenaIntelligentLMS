import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:nldea_capture/core/models/consent_record.dart';
import 'package:nldea_capture/core/models/manifest.dart';
import 'package:nldea_capture/core/providers.dart';
import 'package:nldea_capture/core/services/capture_camera.dart';
import 'package:nldea_capture/core/services/checksums.dart';
import 'package:nldea_capture/core/services/stores.dart';

/// End-to-end session flow at the state layer, camera faked: start ->
/// capture the genuine script -> finalize -> a pipeline-conformant session
/// directory exists on disk.
void main() {
  late Directory tmp;
  late ProviderContainer container;
  late FakeCaptureCamera camera;

  setUp(() async {
    tmp = Directory.systemTemp.createTempSync('nldea_flow_');
    camera = FakeCaptureCamera();
    await camera.initialize();
    container = ProviderContainer(overrides: [
      appDirsProvider.overrideWithValue(AppDirs(tmp)..ensureCreated()),
      captureCameraProvider.overrideWithValue(camera),
    ]);
    container.read(activeConsentProvider.notifier).state = ConsentRecord(
      consentId: 'C-test',
      subjectId: 'NLD-TESTSUBJ',
      consentTextVersion: 'v0-test',
      operatorId: 'OP1',
      agreedAt: DateTime.utc(2026, 8, 5),
    );
  });

  tearDown(() {
    container.dispose();
    tmp.deleteSync(recursive: true);
  });

  test('genuine session produces the exact directory contract', () async {
    final notifier = container.read(activeSessionProvider.notifier);
    final session = notifier.start(
      type: SessionType.genuine,
      lighting: Lighting.lowLight,
      deviceModel: 'Tecno Spark 20',
      skinTone: MonkTone.monk08,
    );

    // The genuine script: neutral, blink, turn_left, turn_right, smile.
    expect(session.plan, [
      null,
      Challenge.blink,
      Challenge.turnLeft,
      Challenge.turnRight,
      Challenge.smile,
    ]);

    for (var i = 0; i < session.plan.length; i++) {
      await notifier.captureClip();
    }
    expect(container.read(activeSessionProvider)!.isComplete, isTrue);

    final manifest = await notifier.finalize();
    expect(container.read(activeSessionProvider), isNull); // draft cleared

    final dir = Directory('${tmp.path}/sessions/${manifest.sessionId}');
    final names = dir
        .listSync()
        .map((e) => e.uri.pathSegments.last)
        .toSet();
    expect(
        names,
        containsAll({
          'clip_001.mp4',
          'clip_002.mp4',
          'clip_003.mp4',
          'clip_004.mp4',
          'clip_005.mp4',
          'manifest.json',
          'checksums.json',
        }));

    final json = (jsonDecode(
            File('${dir.path}/manifest.json').readAsStringSync()) as Map)
        .cast<String, Object?>();
    expect(json['subjectId'], 'NLD-TESTSUBJ');
    expect(json['consentId'], 'C-test');
    expect(json['type'], 'genuine');
    expect(json['attackType'], isNull);
    expect(json['lighting'], 'low_light');
    expect(json['skinTone'], 'monk_08');
    final clips = (json['clips'] as List).cast<Map>();
    expect(clips.map((c) => c['challenge']),
        [null, 'blink', 'turn_left', 'turn_right', 'smile']);
    expect(clips.every((c) => (c['fps'] as num) == 30), isTrue);
    expect(clips.every((c) => (c['durationMs'] as num) > 0), isTrue);

    expect(await verifyChecksums(dir), isEmpty);
  });

  test('retake replaces the clip in place', () async {
    final notifier = container.read(activeSessionProvider.notifier);
    notifier.start(
      type: SessionType.attack,
      attackType: AttackType.replayPhone,
      lighting: Lighting.indoor,
      deviceModel: 'Redmi 13C',
      attackClipCount: 2,
    );
    await notifier.captureClip();
    await notifier.captureClip();
    final before = container.read(activeSessionProvider)!.clips;
    expect(before.length, 2);

    await notifier.captureClip(retakeIndex: 0);
    final after = container.read(activeSessionProvider)!.clips;
    expect(after.length, 2);
    expect(after[0].fileName, 'clip_001.mp4');
    expect(camera.recordedCount, 3);

    final manifest = await notifier.finalize();
    expect(manifest.type, SessionType.attack);
    expect(manifest.attackType, AttackType.replayPhone);
    expect(manifest.skinTone, isNull);
    expect(manifest.clips.length, 2);
    expect(manifest.clips.every((c) => c.challenge == null), isTrue);
  });

  test('discard deletes the working directory', () async {
    final notifier = container.read(activeSessionProvider.notifier);
    final session = notifier.start(
      type: SessionType.genuine,
      lighting: Lighting.daylight,
      deviceModel: 'Samsung Galaxy A15',
    );
    await notifier.captureClip();
    final dir = Directory('${tmp.path}/sessions/${session.sessionId}');
    expect(dir.existsSync(), isTrue);

    await notifier.discard();
    expect(dir.existsSync(), isFalse);
    expect(container.read(activeSessionProvider), isNull);
  });
}
