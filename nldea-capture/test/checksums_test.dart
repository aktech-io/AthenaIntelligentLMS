import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:nldea_capture/core/services/checksums.dart';

void main() {
  late Directory tmp;

  setUp(() {
    tmp = Directory.systemTemp.createTempSync('nldea_checksums_');
  });

  tearDown(() {
    tmp.deleteSync(recursive: true);
  });

  group('checksums', () {
    test('sha256OfFile matches the known digest of "abc"', () async {
      final f = File('${tmp.path}/abc.bin')..writeAsStringSync('abc');
      expect(
        await sha256OfFile(f),
        'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
      );
    });

    test('sidecar covers every file except itself', () async {
      File('${tmp.path}/clip_001.mp4').writeAsStringSync('video-bytes-1');
      File('${tmp.path}/clip_002.mp4').writeAsStringSync('video-bytes-2');
      File('${tmp.path}/manifest.json').writeAsStringSync('{"a":1}');

      final sidecar = await writeChecksumsSidecar(tmp);
      final json = (jsonDecode(await sidecar.readAsString()) as Map)
          .cast<String, Object?>();

      expect(json['algorithm'], 'sha256');
      final files = (json['files'] as Map).cast<String, String>();
      expect(files.keys.toSet(),
          {'clip_001.mp4', 'clip_002.mp4', 'manifest.json'});
      expect(files.containsKey(checksumsFileName), isFalse);
      expect(files['clip_001.mp4'], matches(RegExp(r'^[0-9a-f]{64}$')));
      // Different content, different digest.
      expect(files['clip_001.mp4'], isNot(files['clip_002.mp4']));
    });

    test('verifyChecksums passes on intact dirs, flags tampering', () async {
      File('${tmp.path}/clip_001.mp4').writeAsStringSync('original');
      File('${tmp.path}/manifest.json').writeAsStringSync('{}');
      await writeChecksumsSidecar(tmp);

      expect(await verifyChecksums(tmp), isEmpty);

      File('${tmp.path}/clip_001.mp4').writeAsStringSync('tampered');
      expect(await verifyChecksums(tmp), ['clip_001.mp4']);

      File('${tmp.path}/extra.bin').writeAsStringSync('sneaky');
      expect(await verifyChecksums(tmp), containsAll(['clip_001.mp4', 'extra.bin']));
    });
  });
}
