import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:nldea_capture/core/models/manifest.dart';

void main() {
  group('SessionManifest serialization (pipeline wire contract)', () {
    final genuine = SessionManifest(
      sessionId: '0f8fad5b-d9cb-469f-a165-70867728950e',
      subjectId: 'NLD-7Q2KX9BM',
      consentId: 'C-1c9e9f9a-2c34-4f7a-9a71-111111111111',
      type: SessionType.genuine,
      attackType: null,
      device: const DeviceInfo(model: 'Tecno Spark 20', os: 'android 14'),
      lighting: Lighting.lowLight,
      skinTone: MonkTone.monk08,
      capturedAt: DateTime.utc(2026, 8, 4, 10, 30),
      clips: const [
        ClipEntry(
            file: 'clip_001.mp4', durationMs: 3012, fps: 30, challenge: null),
        ClipEntry(
            file: 'clip_002.mp4',
            durationMs: 2998,
            fps: 30,
            challenge: Challenge.blink),
      ],
    );

    test('emits the exact key set with all keys present', () {
      final json = genuine.toJson();
      expect(
          json.keys.toList(),
          [
            'schemaVersion',
            'sessionId',
            'subjectId',
            'consentId',
            'type',
            'attackType',
            'device',
            'lighting',
            'skinTone',
            'capturedAt',
            'clips',
          ]);
      final clip = (json['clips'] as List).first as Map<String, Object?>;
      expect(clip.keys.toList(), ['file', 'durationMs', 'fps', 'challenge']);
      final device = json['device'] as Map<String, Object?>;
      expect(device.keys.toList(), ['model', 'os']);
    });

    test('genuine session wire values', () {
      final json = genuine.toJson();
      expect(json['schemaVersion'], 1);
      expect(json['type'], 'genuine');
      expect(json['attackType'], isNull);
      expect(json['lighting'], 'low_light');
      expect(json['skinTone'], 'monk_08');
      expect(json['capturedAt'], '2026-08-04T10:30:00.000Z');
      final clips = json['clips'] as List;
      expect((clips[0] as Map)['challenge'], isNull);
      expect((clips[1] as Map)['challenge'], 'blink');
    });

    test('attack session wire values incl. null skinTone', () {
      final attack = SessionManifest(
        sessionId: '0f8fad5b-d9cb-469f-a165-70867728950f',
        subjectId: 'NLD-7Q2KX9BM',
        consentId: 'C-1c9e9f9a-2c34-4f7a-9a71-111111111111',
        type: SessionType.attack,
        attackType: AttackType.replayPhone,
        device: const DeviceInfo(model: 'Redmi 13C', os: 'android 13'),
        lighting: Lighting.indoor,
        skinTone: null,
        capturedAt: DateTime.utc(2026, 8, 4, 11),
        clips: const [
          ClipEntry(
              file: 'clip_001.mp4', durationMs: 3000, fps: 30, challenge: null),
        ],
      );
      final json = attack.toJson();
      expect(json['type'], 'attack');
      expect(json['attackType'], 'replay_phone');
      expect(json.containsKey('skinTone'), isTrue);
      expect(json['skinTone'], isNull);
    });

    test('all attack species use the agreed wire names', () {
      expect(AttackType.values.map((t) => t.wire), [
        'print_flat',
        'print_curved',
        'replay_phone',
        'replay_monitor',
        'cutout_paper',
        'mask_3d',
      ]);
      expect(Lighting.values.map((l) => l.wire),
          ['daylight', 'indoor', 'low_light']);
      expect(Challenge.values.map((c) => c.wire),
          ['blink', 'turn_left', 'turn_right', 'smile']);
      expect(MonkTone.values.first.wire, 'monk_01');
      expect(MonkTone.values.last.wire, 'monk_10');
    });

    test('JSON round-trip preserves every field', () {
      final decoded = SessionManifest.fromJson(
          (jsonDecode(jsonEncode(genuine.toJson())) as Map)
              .cast<String, Object?>());
      expect(decoded.sessionId, genuine.sessionId);
      expect(decoded.subjectId, genuine.subjectId);
      expect(decoded.consentId, genuine.consentId);
      expect(decoded.type, SessionType.genuine);
      expect(decoded.attackType, isNull);
      expect(decoded.device.model, 'Tecno Spark 20');
      expect(decoded.lighting, Lighting.lowLight);
      expect(decoded.skinTone, MonkTone.monk08);
      expect(decoded.capturedAt, genuine.capturedAt);
      expect(decoded.clips.length, 2);
      expect(decoded.clips[1].challenge, Challenge.blink);
      expect(decoded.clips[1].durationMs, 2998);
      expect(decoded.clips[1].fps, 30);
    });

    test('genuine sessions reject a non-null attackType', () {
      expect(
        () => SessionManifest(
          sessionId: 's',
          subjectId: 'NLD-AAAAAAAA',
          consentId: 'c',
          type: SessionType.genuine,
          attackType: AttackType.printFlat,
          device: const DeviceInfo(model: 'm', os: 'o'),
          lighting: Lighting.daylight,
          skinTone: null,
          capturedAt: DateTime.utc(2026),
          clips: const [],
        ),
        throwsA(isA<AssertionError>()),
      );
    });

    test('unknown wire values throw FormatException', () {
      expect(() => SessionType.fromWire('bogus'), throwsFormatException);
      expect(() => AttackType.fromWire('deepfake'), throwsFormatException);
      expect(() => Lighting.fromWire('sunset'), throwsFormatException);
      expect(() => Challenge.fromWire('wave'), throwsFormatException);
      expect(() => MonkTone.fromWire('monk_11'), throwsFormatException);
    });
  });
}
