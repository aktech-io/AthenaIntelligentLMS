import 'dart:convert';
import 'dart:io';

import '../models/consent_record.dart';
import '../models/manifest.dart';
import 'checksums.dart';

/// On-device layout under the app documents directory:
///
/// ```
/// <root>/
///   sessions/<sessionId>/       clips + manifest.json + checksums.json
///   consents/<consentId>.json   consent records — stored SEPARATELY from media
///   exports/                    zipped sessions awaiting hand-off
///   operator.json               last-used operator id (convenience)
/// ```
class AppDirs {
  const AppDirs(this.root);
  final Directory root;

  Directory get sessions => Directory('${root.path}/sessions');
  Directory get consents => Directory('${root.path}/consents');
  Directory get exports => Directory('${root.path}/exports');
  File get operatorFile => File('${root.path}/operator.json');

  void ensureCreated() {
    sessions.createSync(recursive: true);
    consents.createSync(recursive: true);
    exports.createSync(recursive: true);
  }
}

/// Consent records — the only place a subject's consent state lives.
/// No PII: see [ConsentRecord].
///
/// Uses synchronous file IO internally (records are tiny JSON files); the
/// async API is kept so a database-backed implementation can swap in.
/// This also keeps the consent flow usable from widget tests, where real
/// async IO never completes inside the fake-async zone.
class ConsentStore {
  ConsentStore(this.dirs);
  final AppDirs dirs;

  File _fileFor(String consentId) =>
      File('${dirs.consents.path}/$consentId.json');

  Future<void> save(ConsentRecord record) async {
    dirs.ensureCreated();
    _fileFor(record.consentId).writeAsStringSync(
        const JsonEncoder.withIndent('  ').convert(record.toJson()));
  }

  Future<List<ConsentRecord>> loadAll() async {
    if (!dirs.consents.existsSync()) return [];
    final records = <ConsentRecord>[];
    for (final f in dirs.consents.listSync().whereType<File>()) {
      if (!f.path.endsWith('.json')) continue;
      records.add(ConsentRecord.fromJson(
          (jsonDecode(f.readAsStringSync()) as Map).cast<String, Object?>()));
    }
    records.sort((a, b) => a.agreedAt.compareTo(b.agreedAt));
    return records;
  }

  Future<List<ConsentRecord>> forSubject(String subjectId) async =>
      (await loadAll())
          .where((r) => r.subjectId == subjectId.trim().toUpperCase())
          .toList();

  /// True if the pseudonym is already in use (defensive; collisions are
  /// astronomically unlikely).
  Future<bool> subjectIdExists(String subjectId) async =>
      (await forSubject(subjectId)).isNotEmpty;

  /// Marks every consent record for [subjectId] withdrawn. Returns the
  /// updated records (empty = unknown subject).
  Future<List<ConsentRecord>> markWithdrawn(String subjectId,
      {DateTime? at}) async {
    final when = at ?? DateTime.now().toUtc();
    final updated = <ConsentRecord>[];
    for (final record in await forSubject(subjectId)) {
      final w = record.isWithdrawn ? record : record.withdrawn(when);
      await save(w);
      updated.add(w);
    }
    return updated;
  }
}

/// Finalized capture sessions on disk. One directory per session, named by
/// sessionId, containing clips + manifest.json + checksums.json.
class SessionStore {
  SessionStore(this.dirs);
  final AppDirs dirs;

  Directory sessionDir(String sessionId) =>
      Directory('${dirs.sessions.path}/$sessionId');

  /// Creates the (empty) working directory for an in-progress session.
  Directory createSessionDir(String sessionId) {
    dirs.ensureCreated();
    return sessionDir(sessionId)..createSync(recursive: true);
  }

  /// Finalizes a session: writes `manifest.json`, then the `checksums.json`
  /// sidecar covering every file in the directory.
  Future<void> finalize(SessionManifest manifest) async {
    final dir = sessionDir(manifest.sessionId);
    if (!dir.existsSync()) {
      throw StateError('Session directory missing: ${dir.path}');
    }
    await File('${dir.path}/manifest.json').writeAsString(
        const JsonEncoder.withIndent('  ').convert(manifest.toJson()));
    await writeChecksumsSidecar(dir);
  }

  /// All finalized sessions (directories with a parseable manifest.json).
  Future<List<SessionManifest>> loadManifests() async {
    if (!dirs.sessions.existsSync()) return [];
    final manifests = <SessionManifest>[];
    for (final dir in dirs.sessions.listSync().whereType<Directory>()) {
      final f = File('${dir.path}/manifest.json');
      if (!f.existsSync()) continue; // in-progress or aborted session
      try {
        manifests.add(SessionManifest.fromJson(
            (jsonDecode(await f.readAsString()) as Map)
                .cast<String, Object?>()));
      } on FormatException {
        // Corrupt manifest: skip, surfaced by checksum verification instead.
      }
    }
    manifests.sort((a, b) => a.capturedAt.compareTo(b.capturedAt));
    return manifests;
  }

  Future<List<SessionManifest>> sessionsForSubject(String subjectId) async =>
      (await loadManifests())
          .where((m) => m.subjectId == subjectId.trim().toUpperCase())
          .toList();

  /// Deletes a session directory and all media in it (withdrawal path).
  Future<void> deleteSession(String sessionId) async {
    final dir = sessionDir(sessionId);
    if (dir.existsSync()) await dir.delete(recursive: true);
  }

  /// Discards an in-progress (never finalized) session directory.
  Future<void> discard(String sessionId) => deleteSession(sessionId);
}
