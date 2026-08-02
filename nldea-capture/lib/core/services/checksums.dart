import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';

/// Integrity sidecar: every finalized session directory gets a
/// `checksums.json` next to `manifest.json`:
///
/// ```json
/// {"algorithm":"sha256","files":{"clip_001.mp4":"<hex>","manifest.json":"<hex>"}}
/// ```
///
/// Every file in the session directory except the sidecar itself is hashed
/// (clips, manifest, review thumbnails), so the pipeline can verify a
/// transferred session byte-for-byte.
const String checksumsFileName = 'checksums.json';

/// Streaming SHA-256 of a file, lowercase hex.
Future<String> sha256OfFile(File file) async {
  final digest = await sha256.bind(file.openRead()).first;
  return digest.toString();
}

/// Hashes every regular file in [dir] (non-recursive) except
/// `checksums.json`, returning `relative file name -> hex digest`,
/// sorted by name for stable output.
Future<Map<String, String>> computeDirectoryChecksums(Directory dir) async {
  final entries = <String, String>{};
  final files = dir
      .listSync()
      .whereType<File>()
      .where((f) => f.uri.pathSegments.last != checksumsFileName)
      .toList()
    ..sort((a, b) => a.path.compareTo(b.path));
  for (final file in files) {
    entries[file.uri.pathSegments.last] = await sha256OfFile(file);
  }
  return entries;
}

/// Computes and writes the `checksums.json` sidecar for a session directory.
Future<File> writeChecksumsSidecar(Directory sessionDir) async {
  final files = await computeDirectoryChecksums(sessionDir);
  final sidecar = File('${sessionDir.path}/$checksumsFileName');
  await sidecar.writeAsString(const JsonEncoder.withIndent('  ').convert({
    'algorithm': 'sha256',
    'files': files,
  }));
  return sidecar;
}

/// Re-hashes [sessionDir] and compares against its sidecar.
/// Returns the list of mismatched/missing file names (empty = intact).
Future<List<String>> verifyChecksums(Directory sessionDir) async {
  final sidecar = File('${sessionDir.path}/$checksumsFileName');
  if (!sidecar.existsSync()) return [checksumsFileName];
  final recorded = ((jsonDecode(await sidecar.readAsString())
          as Map)['files'] as Map)
      .cast<String, String>();
  final actual = await computeDirectoryChecksums(sessionDir);
  final bad = <String>[];
  for (final entry in recorded.entries) {
    if (actual[entry.key] != entry.value) bad.add(entry.key);
  }
  for (final name in actual.keys) {
    if (!recorded.containsKey(name)) bad.add(name);
  }
  return bad;
}
