import 'dart:convert';
import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'models/consent_record.dart';
import 'models/manifest.dart';
import 'services/campaign.dart';
import 'services/capture_camera.dart';
import 'services/export_service.dart';
import 'services/ids.dart';
import 'services/stores.dart';

/// Root storage. Overridden at bootstrap (main.dart resolves the app
/// documents directory via path_provider) and in tests (temp dir).
final appDirsProvider = Provider<AppDirs>(
  (ref) => throw UnimplementedError(
      'appDirsProvider must be overridden at bootstrap'),
);

final consentStoreProvider =
    Provider<ConsentStore>((ref) => ConsentStore(ref.watch(appDirsProvider)));

final sessionStoreProvider =
    Provider<SessionStore>((ref) => SessionStore(ref.watch(appDirsProvider)));

final exportServiceProvider =
    Provider<ExportService>((ref) => ExportService(ref.watch(appDirsProvider)));

/// PHASE-2 STUB — swaps to the real on-prem uploader when the backend exists.
final uploadServiceProvider =
    Provider<UploadService>((ref) => StubUploadService());

/// Camera seam. Overridden with [FakeCaptureCamera] in tests/dev.
final captureCameraProvider =
    Provider<CaptureCamera>((ref) => PluginCaptureCamera());

final campaignTargetsProvider =
    Provider<CampaignTargets>((ref) => defaultTargets);

/// Campaign progress, recomputed on [sessionsChangedProvider] bumps.
final campaignStatsProvider = FutureProvider<CampaignStats>((ref) async {
  ref.watch(sessionsChangedProvider);
  final manifests = await ref.watch(sessionStoreProvider).loadManifests();
  return CampaignStats.fromManifests(manifests);
});

/// Bumped whenever sessions are finalized/deleted so listings refresh.
final sessionsChangedProvider = StateProvider<int>((ref) => 0);

final allManifestsProvider = FutureProvider<List<SessionManifest>>((ref) {
  ref.watch(sessionsChangedProvider);
  return ref.watch(sessionStoreProvider).loadManifests();
});

/// Operator id (initials/staff code) persisted across sessions.
final operatorIdProvider =
    NotifierProvider<OperatorIdNotifier, String>(OperatorIdNotifier.new);

class OperatorIdNotifier extends Notifier<String> {
  @override
  String build() {
    final f = ref.watch(appDirsProvider).operatorFile;
    if (f.existsSync()) {
      try {
        return (jsonDecode(f.readAsStringSync())
                as Map)['operatorId'] as String? ??
            '';
      } on FormatException {
        return '';
      }
    }
    return '';
  }

  void set(String id) {
    state = id.trim();
    final f = ref.read(appDirsProvider).operatorFile;
    f.parent.createSync(recursive: true);
    f.writeAsStringSync(jsonEncode({'operatorId': state}));
  }
}

// ---------------------------------------------------------------------------
// Active consent + in-progress session
// ---------------------------------------------------------------------------

/// THE consent gate. Null until the subject has agreed on the consent
/// screen; the router redirects every capture route to /consent while null.
final activeConsentProvider = StateProvider<ConsentRecord?>((ref) => null);

/// One clip captured into the working session directory.
class PendingClip {
  const PendingClip({
    required this.fileName,
    required this.filePath,
    required this.durationMs,
    required this.fps,
    required this.challenge,
    this.thumbnailPath,
  });

  final String fileName;
  final String filePath;
  final int durationMs;
  final int fps;
  final Challenge? challenge;
  final String? thumbnailPath;

  ClipEntry toEntry() => ClipEntry(
      file: fileName, durationMs: durationMs, fps: fps, challenge: challenge);
}

/// Draft session being configured/captured. Immutable value object.
class ActiveSession {
  const ActiveSession({
    required this.sessionId,
    required this.type,
    this.attackType,
    required this.lighting,
    required this.deviceModel,
    this.skinTone,
    this.attackClipCount = 4,
    this.clips = const [],
  });

  final String sessionId;
  final SessionType type;
  final AttackType? attackType;
  final Lighting lighting;
  final String deviceModel;
  final MonkTone? skinTone;

  /// Number of clips to capture in attack mode (genuine mode always runs
  /// the full 5-step challenge script).
  final int attackClipCount;
  final List<PendingClip> clips;

  /// The scripted capture plan: challenge per clip index (null = neutral).
  List<Challenge?> get plan => type == SessionType.genuine
      ? const [
          null, // neutral hold
          Challenge.blink,
          Challenge.turnLeft,
          Challenge.turnRight,
          Challenge.smile,
        ]
      : List<Challenge?>.filled(attackClipCount, null);

  bool get isComplete => clips.length >= plan.length;

  ActiveSession copyWith({
    SessionType? type,
    AttackType? attackType,
    bool clearAttackType = false,
    Lighting? lighting,
    String? deviceModel,
    MonkTone? skinTone,
    bool clearSkinTone = false,
    int? attackClipCount,
    List<PendingClip>? clips,
  }) =>
      ActiveSession(
        sessionId: sessionId,
        type: type ?? this.type,
        attackType: clearAttackType ? null : (attackType ?? this.attackType),
        lighting: lighting ?? this.lighting,
        deviceModel: deviceModel ?? this.deviceModel,
        skinTone: clearSkinTone ? null : (skinTone ?? this.skinTone),
        attackClipCount: attackClipCount ?? this.attackClipCount,
        clips: clips ?? this.clips,
      );
}

final activeSessionProvider =
    NotifierProvider<ActiveSessionNotifier, ActiveSession?>(
        ActiveSessionNotifier.new);

class ActiveSessionNotifier extends Notifier<ActiveSession?> {
  @override
  ActiveSession? build() => null;

  /// Starts a new draft (consent must already be granted; enforced by the
  /// router gate). Creates the working directory immediately.
  ActiveSession start({
    required SessionType type,
    AttackType? attackType,
    required Lighting lighting,
    required String deviceModel,
    MonkTone? skinTone,
    int attackClipCount = 4,
  }) {
    final session = ActiveSession(
      sessionId: generateSessionId(),
      type: type,
      attackType: type == SessionType.attack ? attackType : null,
      lighting: lighting,
      deviceModel: deviceModel,
      skinTone: skinTone,
      attackClipCount: attackClipCount,
    );
    ref.read(sessionStoreProvider).createSessionDir(session.sessionId);
    state = session;
    return session;
  }

  void update(ActiveSession session) => state = session;

  String get _dirPath => ref
      .read(sessionStoreProvider)
      .sessionDir(state!.sessionId)
      .path;

  /// Records the next clip in the plan (or retakes [retakeIndex]).
  Future<void> captureClip({int? retakeIndex}) async {
    final session = state;
    if (session == null) throw StateError('No active session');
    final index = retakeIndex ?? session.clips.length;
    if (index >= session.plan.length) throw StateError('Plan complete');

    final fileName = 'clip_${(index + 1).toString().padLeft(3, '0')}.mp4';
    final camera = ref.read(captureCameraProvider);
    final recorded = await camera.recordClip(
      duration: const Duration(seconds: 3),
      outputPath: '$_dirPath/$fileName',
    );
    final clip = PendingClip(
      fileName: fileName,
      filePath: recorded.filePath,
      durationMs: recorded.durationMs,
      fps: recorded.fps,
      challenge: session.plan[index],
      thumbnailPath: recorded.thumbnailPath,
    );
    final clips = [...session.clips];
    if (index < clips.length) {
      clips[index] = clip;
    } else {
      clips.add(clip);
    }
    state = session.copyWith(clips: clips);
  }

  /// Writes manifest.json + checksums.json and clears the draft.
  /// Returns the finalized manifest.
  Future<SessionManifest> finalize() async {
    final session = state;
    final consent = ref.read(activeConsentProvider);
    if (session == null || consent == null) {
      throw StateError('No active session/consent');
    }
    final manifest = SessionManifest(
      sessionId: session.sessionId,
      subjectId: consent.subjectId,
      consentId: consent.consentId,
      type: session.type,
      attackType: session.attackType,
      device: DeviceInfo(
        model: session.deviceModel,
        os: '${Platform.operatingSystem} ${Platform.operatingSystemVersion}',
      ),
      lighting: session.lighting,
      skinTone: session.skinTone,
      capturedAt: DateTime.now().toUtc(),
      clips: session.clips.map((c) => c.toEntry()).toList(),
    );
    await ref.read(sessionStoreProvider).finalize(manifest);
    state = null;
    ref.read(sessionsChangedProvider.notifier).state++;
    return manifest;
  }

  /// Aborts the draft and deletes any captured media.
  Future<void> discard() async {
    final session = state;
    if (session == null) return;
    await ref.read(sessionStoreProvider).discard(session.sessionId);
    state = null;
  }
}
