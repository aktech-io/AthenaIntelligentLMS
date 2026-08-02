import 'package:flutter_test/flutter_test.dart';
import 'package:nldea_capture/core/models/manifest.dart';
import 'package:nldea_capture/core/services/campaign.dart';

SessionManifest _session({
  required String subject,
  required SessionType type,
  AttackType? attackType,
  Lighting lighting = Lighting.indoor,
  MonkTone? tone,
  int clipCount = 5,
}) =>
    SessionManifest(
      sessionId: 'sess-$subject-${type.wire}-${attackType?.wire}-'
          '${lighting.wire}-$clipCount',
      subjectId: subject,
      consentId: 'C-x',
      type: type,
      attackType: attackType,
      device: const DeviceInfo(model: 'Tecno Spark 20', os: 'android 14'),
      lighting: lighting,
      skinTone: tone,
      capturedAt: DateTime.utc(2026, 8, 10),
      clips: [
        for (var i = 0; i < clipCount; i++)
          ClipEntry(
              file: 'clip_${i + 1}.mp4',
              durationMs: 3000,
              fps: 30,
              challenge: null),
      ],
    );

void main() {
  group('CampaignStats', () {
    test('counts subjects once across multiple sessions', () {
      final stats = CampaignStats.fromManifests([
        _session(subject: 'NLD-AAAAAAAA', type: SessionType.genuine),
        _session(
            subject: 'NLD-AAAAAAAA',
            type: SessionType.genuine,
            lighting: Lighting.daylight),
        _session(subject: 'NLD-BBBBBBBB', type: SessionType.genuine),
      ]);
      expect(stats.subjectCount, 2);
      expect(stats.sessionCount, 3);
      expect(stats.genuineClipCount, 15);
    });

    test('splits attack clips by species and never mixes into genuine', () {
      final stats = CampaignStats.fromManifests([
        _session(subject: 'NLD-AAAAAAAA', type: SessionType.genuine),
        _session(
            subject: 'NLD-AAAAAAAA',
            type: SessionType.attack,
            attackType: AttackType.replayPhone,
            clipCount: 4),
        _session(
            subject: 'NLD-BBBBBBBB',
            type: SessionType.attack,
            attackType: AttackType.printFlat,
            clipCount: 3),
        _session(
            subject: 'NLD-BBBBBBBB',
            type: SessionType.attack,
            attackType: AttackType.replayPhone,
            clipCount: 2),
      ]);
      expect(stats.genuineClipCount, 5);
      expect(stats.attackClipCounts[AttackType.replayPhone], 6);
      expect(stats.attackClipCounts[AttackType.printFlat], 3);
      expect(stats.attackClipCounts[AttackType.mask3d], 0);
      expect(stats.totalAttackClipCount, 9);
    });

    test('lighting breakdown counts clips per station', () {
      final stats = CampaignStats.fromManifests([
        _session(
            subject: 'NLD-AAAAAAAA',
            type: SessionType.genuine,
            lighting: Lighting.lowLight,
            clipCount: 5),
        _session(
            subject: 'NLD-AAAAAAAA',
            type: SessionType.genuine,
            lighting: Lighting.daylight,
            clipCount: 5),
      ]);
      expect(stats.sessionsByLighting[Lighting.lowLight], 5);
      expect(stats.sessionsByLighting[Lighting.daylight], 5);
      expect(stats.sessionsByLighting[Lighting.indoor], 0);
      expect(stats.genuineClipsByLighting[Lighting.lowLight], 5);
    });

    test('dark-tone share tracks the >=60% Monk 7-10 quota', () {
      final stats = CampaignStats.fromManifests([
        _session(
            subject: 'NLD-AAAAAAAA',
            type: SessionType.genuine,
            tone: MonkTone.monk08,
            clipCount: 6),
        _session(
            subject: 'NLD-BBBBBBBB',
            type: SessionType.genuine,
            tone: MonkTone.monk03,
            clipCount: 4),
        // No tone report: excluded from the share entirely.
        _session(
            subject: 'NLD-CCCCCCCC',
            type: SessionType.genuine,
            clipCount: 5),
      ]);
      expect(stats.darkToneShare, closeTo(0.6, 1e-9));
      expect(stats.skinToneClipCounts[MonkTone.monk08], 6);
      expect(stats.skinToneClipCounts[MonkTone.monk03], 4);
    });

    test('darkToneShare is null before any tone reports', () {
      final stats = CampaignStats.fromManifests(
          [_session(subject: 'NLD-AAAAAAAA', type: SessionType.genuine)]);
      expect(stats.darkToneShare, isNull);
    });

    test('gate G2 fires at the configured subject count', () {
      const targets = CampaignTargets(g2Subjects: 2);
      final one = CampaignStats.fromManifests(
          [_session(subject: 'NLD-AAAAAAAA', type: SessionType.genuine)]);
      final two = CampaignStats.fromManifests([
        _session(subject: 'NLD-AAAAAAAA', type: SessionType.genuine),
        _session(subject: 'NLD-BBBBBBBB', type: SessionType.genuine),
      ]);
      expect(one.g2Reached(targets), isFalse);
      expect(two.g2Reached(targets), isTrue);
    });

    test('default targets match the doc-10 campaign design', () {
      expect(defaultTargets.totalSubjects, 400);
      expect(defaultTargets.g2Subjects, 200);
      expect(defaultTargets.genuineClips, 3200);
      expect(defaultTargets.attackClips[AttackType.replayPhone], 500);
      expect(defaultTargets.attackClips[AttackType.mask3d], 0); // L2 phase
      expect(defaultTargets.totalAttackClips, 1800);
    });
  });
}
