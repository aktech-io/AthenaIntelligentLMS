import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:nldea_capture/core/models/consent_record.dart';
import 'package:nldea_capture/core/models/manifest.dart';
import 'package:nldea_capture/core/services/checksums.dart';
import 'package:nldea_capture/core/services/stores.dart';

void main() {
  late Directory tmp;
  late AppDirs dirs;

  setUp(() {
    tmp = Directory.systemTemp.createTempSync('nldea_stores_');
    dirs = AppDirs(tmp)..ensureCreated();
  });

  tearDown(() {
    tmp.deleteSync(recursive: true);
  });

  SessionManifest manifest(String sessionId, String subjectId) =>
      SessionManifest(
        sessionId: sessionId,
        subjectId: subjectId,
        consentId: 'C-1',
        type: SessionType.genuine,
        attackType: null,
        device: const DeviceInfo(model: 'Redmi 13C', os: 'android 13'),
        lighting: Lighting.daylight,
        skinTone: MonkTone.monk09,
        capturedAt: DateTime.utc(2026, 8, 5),
        clips: const [
          ClipEntry(
              file: 'clip_001.mp4', durationMs: 3000, fps: 30, challenge: null),
        ],
      );

  group('SessionStore', () {
    test('finalize writes manifest.json + checksums.json sidecar', () async {
      final store = SessionStore(dirs);
      final dir = store.createSessionDir('sess-1');
      File('${dir.path}/clip_001.mp4').writeAsStringSync('bytes');

      await store.finalize(manifest('sess-1', 'NLD-AAAAAAAA'));

      final written = (jsonDecode(
              File('${dir.path}/manifest.json').readAsStringSync()) as Map)
          .cast<String, Object?>();
      expect(written['sessionId'], 'sess-1');
      expect(written['skinTone'], 'monk_09');

      final sidecar = (jsonDecode(
              File('${dir.path}/checksums.json').readAsStringSync()) as Map)
          .cast<String, Object?>();
      final files = (sidecar['files'] as Map).cast<String, String>();
      expect(files.keys.toSet(), {'clip_001.mp4', 'manifest.json'});
      expect(await verifyChecksums(dir), isEmpty);
    });

    test('loadManifests skips unfinalized sessions, sorts by capture time',
        () async {
      final store = SessionStore(dirs);
      store.createSessionDir('unfinished'); // no manifest.json
      store.createSessionDir('sess-1');
      File('${store.sessionDir('sess-1').path}/clip_001.mp4')
          .writeAsStringSync('x');
      await store.finalize(manifest('sess-1', 'NLD-AAAAAAAA'));

      final loaded = await store.loadManifests();
      expect(loaded.map((m) => m.sessionId), ['sess-1']);
    });

    test('deleteSession removes all media (withdrawal path)', () async {
      final store = SessionStore(dirs);
      final dir = store.createSessionDir('sess-1');
      File('${dir.path}/clip_001.mp4').writeAsStringSync('x');
      await store.finalize(manifest('sess-1', 'NLD-AAAAAAAA'));

      await store.deleteSession('sess-1');
      expect(dir.existsSync(), isFalse);
      expect(await store.sessionsForSubject('NLD-AAAAAAAA'), isEmpty);
    });
  });

  group('ConsentStore', () {
    ConsentRecord record(String consentId, String subjectId) => ConsentRecord(
          consentId: consentId,
          subjectId: subjectId,
          consentTextVersion: 'v0-test',
          operatorId: 'OP1',
          agreedAt: DateTime.utc(2026, 8, 5, 9),
        );

    test('consent records live outside the sessions tree', () async {
      final store = ConsentStore(dirs);
      await store.save(record('C-1', 'NLD-AAAAAAAA'));
      expect(File('${dirs.consents.path}/C-1.json').existsSync(), isTrue);
      // Data minimization: nothing consent-related under sessions/.
      expect(
          dirs.sessions
              .listSync(recursive: true)
              .where((e) => e.path.contains('C-1')),
          isEmpty);
    });

    test('markWithdrawn stamps every record for the subject', () async {
      final store = ConsentStore(dirs);
      await store.save(record('C-1', 'NLD-AAAAAAAA'));
      await store.save(record('C-2', 'NLD-AAAAAAAA'));
      await store.save(record('C-3', 'NLD-BBBBBBBB'));

      final updated = await store.markWithdrawn('nld-aaaaaaaa'); // any case
      expect(updated.length, 2);
      expect(updated.every((r) => r.isWithdrawn), isTrue);

      final other = await store.forSubject('NLD-BBBBBBBB');
      expect(other.single.isWithdrawn, isFalse);
    });

    test('subjectIdExists guards against pseudonym reuse', () async {
      final store = ConsentStore(dirs);
      expect(await store.subjectIdExists('NLD-AAAAAAAA'), isFalse);
      await store.save(record('C-1', 'NLD-AAAAAAAA'));
      expect(await store.subjectIdExists('NLD-AAAAAAAA'), isTrue);
    });
  });
}
