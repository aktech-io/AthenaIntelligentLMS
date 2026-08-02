/// NLD-EA session manifest — the wire contract shared with the training
/// pipeline and the red-team rig. Every finalized session directory contains
/// clips plus a `manifest.json` that serializes [SessionManifest] EXACTLY:
///
/// ```json
/// {"schemaVersion":1,"sessionId":"<uuid>","subjectId":"<pseudonymous-id>",
///  "consentId":"<id>","type":"genuine"|"attack",
///  "attackType":null|"print_flat"|"print_curved"|"replay_phone"|
///                "replay_monitor"|"cutout_paper"|"mask_3d",
///  "device":{"model":"<str>","os":"<str>"},
///  "lighting":"daylight"|"indoor"|"low_light",
///  "skinTone":"monk_01".."monk_10"|null,
///  "capturedAt":"<iso8601>",
///  "clips":[{"file":"clip_001.mp4","durationMs":0,"fps":0,
///            "challenge":null|"blink"|"turn_left"|"turn_right"|"smile"}]}
/// ```
///
/// Do not rename wire values without coordinating with the pipeline owners.
library;

/// Wire value of `type`.
enum SessionType {
  genuine('genuine'),
  attack('attack');

  const SessionType(this.wire);
  final String wire;

  static SessionType fromWire(String value) =>
      values.firstWhere((v) => v.wire == value,
          orElse: () => throw FormatException('Unknown session type: $value'));
}

/// Wire value of `attackType` (null for genuine sessions).
enum AttackType {
  printFlat('print_flat', 'Print — flat (matte)'),
  printCurved('print_curved', 'Print — curved (glossy)'),
  replayPhone('replay_phone', 'Replay — phone screen'),
  replayMonitor('replay_monitor', 'Replay — monitor'),
  cutoutPaper('cutout_paper', 'Cutout — paper mask'),

  /// Present in the schema for forward-compatibility; capture is deferred to
  /// the Level-2 phase (3D mask fabrication, doc 06 §7b).
  mask3d('mask_3d', '3D mask (L2 phase)');

  const AttackType(this.wire, this.label);
  final String wire;
  final String label;

  static AttackType fromWire(String value) =>
      values.firstWhere((v) => v.wire == value,
          orElse: () => throw FormatException('Unknown attack type: $value'));
}

/// Wire value of `lighting` — the three campaign lighting stations
/// (daylight >1,000 lux · office ~300 lux · dim <50 lux).
enum Lighting {
  daylight('daylight', 'Daylight (>1,000 lux)'),
  indoor('indoor', 'Indoor office (~300 lux)'),
  lowLight('low_light', 'Low light (<50 lux)');

  const Lighting(this.wire, this.label);
  final String wire;
  final String label;

  static Lighting fromWire(String value) =>
      values.firstWhere((v) => v.wire == value,
          orElse: () => throw FormatException('Unknown lighting: $value'));
}

/// Wire value of a clip's `challenge` (null = neutral hold).
enum Challenge {
  blink('blink', 'Blink twice'),
  turnLeft('turn_left', 'Turn head LEFT'),
  turnRight('turn_right', 'Turn head RIGHT'),
  smile('smile', 'Smile');

  const Challenge(this.wire, this.label);
  final String wire;
  final String label;

  static Challenge fromWire(String value) =>
      values.firstWhere((v) => v.wire == value,
          orElse: () => throw FormatException('Unknown challenge: $value'));
}

/// Monk skin-tone scale self-report, `monk_01`..`monk_10` (null = declined).
/// Hex swatches are approximations of the published Monk scale, good enough
/// for operator-facing UI; the wire value is what the pipeline consumes.
enum MonkTone {
  monk01('monk_01', 0xFFF6EDE4),
  monk02('monk_02', 0xFFF3E7DB),
  monk03('monk_03', 0xFFF7EAD0),
  monk04('monk_04', 0xFFEADABA),
  monk05('monk_05', 0xFFD7BD96),
  monk06('monk_06', 0xFFA07E56),
  monk07('monk_07', 0xFF825C43),
  monk08('monk_08', 0xFF604134),
  monk09('monk_09', 0xFF3A312A),
  monk10('monk_10', 0xFF292420);

  const MonkTone(this.wire, this.argb);
  final String wire;

  /// Approximate swatch color as 0xAARRGGBB.
  final int argb;

  static MonkTone fromWire(String value) =>
      values.firstWhere((v) => v.wire == value,
          orElse: () => throw FormatException('Unknown Monk tone: $value'));
}

/// `device` object: the capture device (fleet handset), recorded per session.
class DeviceInfo {
  const DeviceInfo({required this.model, required this.os});

  final String model;
  final String os;

  Map<String, Object?> toJson() => {'model': model, 'os': os};

  factory DeviceInfo.fromJson(Map<String, Object?> json) => DeviceInfo(
        model: json['model'] as String,
        os: json['os'] as String,
      );
}

/// One entry of the manifest `clips` array.
class ClipEntry {
  const ClipEntry({
    required this.file,
    required this.durationMs,
    required this.fps,
    required this.challenge,
  });

  /// File name relative to the session directory, e.g. `clip_001.mp4`.
  final String file;
  final int durationMs;

  /// Target capture fps (the recorder's configured rate).
  final int fps;
  final Challenge? challenge;

  Map<String, Object?> toJson() => {
        'file': file,
        'durationMs': durationMs,
        'fps': fps,
        'challenge': challenge?.wire,
      };

  factory ClipEntry.fromJson(Map<String, Object?> json) => ClipEntry(
        file: json['file'] as String,
        durationMs: (json['durationMs'] as num).toInt(),
        fps: (json['fps'] as num).toInt(),
        challenge: json['challenge'] == null
            ? null
            : Challenge.fromWire(json['challenge'] as String),
      );
}

/// The `manifest.json` contract. All keys are always present; nullable
/// fields serialize as JSON null.
class SessionManifest {
  const SessionManifest({
    this.schemaVersion = 1,
    required this.sessionId,
    required this.subjectId,
    required this.consentId,
    required this.type,
    required this.attackType,
    required this.device,
    required this.lighting,
    required this.skinTone,
    required this.capturedAt,
    required this.clips,
  }) : assert(type == SessionType.attack || attackType == null,
            'attackType must be null for genuine sessions');

  final int schemaVersion;
  final String sessionId;
  final String subjectId;
  final String consentId;
  final SessionType type;
  final AttackType? attackType;
  final DeviceInfo device;
  final Lighting lighting;
  final MonkTone? skinTone;
  final DateTime capturedAt;
  final List<ClipEntry> clips;

  Map<String, Object?> toJson() => {
        'schemaVersion': schemaVersion,
        'sessionId': sessionId,
        'subjectId': subjectId,
        'consentId': consentId,
        'type': type.wire,
        'attackType': attackType?.wire,
        'device': device.toJson(),
        'lighting': lighting.wire,
        'skinTone': skinTone?.wire,
        'capturedAt': capturedAt.toUtc().toIso8601String(),
        'clips': clips.map((c) => c.toJson()).toList(),
      };

  factory SessionManifest.fromJson(Map<String, Object?> json) =>
      SessionManifest(
        schemaVersion: (json['schemaVersion'] as num).toInt(),
        sessionId: json['sessionId'] as String,
        subjectId: json['subjectId'] as String,
        consentId: json['consentId'] as String,
        type: SessionType.fromWire(json['type'] as String),
        attackType: json['attackType'] == null
            ? null
            : AttackType.fromWire(json['attackType'] as String),
        device: DeviceInfo.fromJson(
            (json['device'] as Map).cast<String, Object?>()),
        lighting: Lighting.fromWire(json['lighting'] as String),
        skinTone: json['skinTone'] == null
            ? null
            : MonkTone.fromWire(json['skinTone'] as String),
        capturedAt: DateTime.parse(json['capturedAt'] as String),
        clips: (json['clips'] as List)
            .map((c) => ClipEntry.fromJson((c as Map).cast<String, Object?>()))
            .toList(),
      );
}
