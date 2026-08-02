import '../models/manifest.dart';

/// Campaign targets from docs/nemo/10-liveness-stage0-and-data-campaign.md.
/// Local config for v1; a later phase can load these from a server.
class CampaignTargets {
  const CampaignTargets({
    this.totalSubjects = 400,
    this.g2Subjects = 200,
    this.genuineClips = 3200,
    this.attackClips = const {
      // matte print -> print_flat, glossy print -> print_curved
      AttackType.printFlat: 400,
      AttackType.printCurved: 300,
      AttackType.cutoutPaper: 300,
      AttackType.replayPhone: 500,
      AttackType.replayMonitor: 300,
      AttackType.mask3d: 0, // L2 phase — not part of the v1 campaign
    },
    this.minDarkToneShare = 0.60,
  });

  final int totalSubjects;

  /// Gate G2 (week 5): first teacher fine-tune triggers here.
  final int g2Subjects;
  final int genuineClips;
  final Map<AttackType, int> attackClips;

  /// Quota: >=60% of skin-tone reports in Monk 7-10.
  final double minDarkToneShare;

  int get totalAttackClips =>
      attackClips.values.fold(0, (sum, n) => sum + n);
}

const CampaignTargets defaultTargets = CampaignTargets();

/// Aggregated progress over a set of finalized session manifests, for the
/// operator dashboard (gate G2 visibility).
class CampaignStats {
  CampaignStats._({
    required this.subjectCount,
    required this.genuineClipCount,
    required this.attackClipCounts,
    required this.sessionsByLighting,
    required this.genuineClipsByLighting,
    required this.skinToneClipCounts,
    required this.sessionCount,
  });

  factory CampaignStats.fromManifests(Iterable<SessionManifest> manifests) {
    final subjects = <String>{};
    var genuineClips = 0;
    var sessions = 0;
    final attackCounts = <AttackType, int>{
      for (final t in AttackType.values) t: 0,
    };
    final byLighting = <Lighting, int>{for (final l in Lighting.values) l: 0};
    final genuineByLighting = <Lighting, int>{
      for (final l in Lighting.values) l: 0,
    };
    final byTone = <MonkTone, int>{for (final t in MonkTone.values) t: 0};

    for (final m in manifests) {
      sessions++;
      subjects.add(m.subjectId);
      byLighting[m.lighting] = byLighting[m.lighting]! + m.clips.length;
      if (m.skinTone != null) {
        byTone[m.skinTone!] = byTone[m.skinTone!]! + m.clips.length;
      }
      if (m.type == SessionType.genuine) {
        genuineClips += m.clips.length;
        genuineByLighting[m.lighting] =
            genuineByLighting[m.lighting]! + m.clips.length;
      } else if (m.attackType != null) {
        attackCounts[m.attackType!] =
            attackCounts[m.attackType!]! + m.clips.length;
      }
    }
    return CampaignStats._(
      subjectCount: subjects.length,
      genuineClipCount: genuineClips,
      attackClipCounts: attackCounts,
      sessionsByLighting: byLighting,
      genuineClipsByLighting: genuineByLighting,
      skinToneClipCounts: byTone,
      sessionCount: sessions,
    );
  }

  final int subjectCount;
  final int sessionCount;
  final int genuineClipCount;
  final Map<AttackType, int> attackClipCounts;

  /// Clips per lighting station (all session types).
  final Map<Lighting, int> sessionsByLighting;
  final Map<Lighting, int> genuineClipsByLighting;

  /// Clips per self-reported Monk tone (sessions without a report excluded).
  final Map<MonkTone, int> skinToneClipCounts;

  int get totalAttackClipCount =>
      attackClipCounts.values.fold(0, (sum, n) => sum + n);

  /// Share of tone-reported clips in Monk 7-10 (the >=60% quota axis).
  /// Null when nothing has a skin-tone report yet.
  double? get darkToneShare {
    final total = skinToneClipCounts.values.fold(0, (s, n) => s + n);
    if (total == 0) return null;
    final dark = MonkTone.values
        .where((t) => t.index >= MonkTone.monk07.index)
        .fold(0, (s, t) => s + skinToneClipCounts[t]!);
    return dark / total;
  }

  bool g2Reached(CampaignTargets targets) =>
      subjectCount >= targets.g2Subjects;
}
