import 'dart:io';

import 'package:archive/archive_io.dart';
import 'package:share_plus/share_plus.dart';

import 'stores.dart';

/// v1 export: zip finalized session directories into `<root>/exports/` and
/// hand off via the platform share sheet (USB/Drive/etc.). The upload
/// service below is the phase-2 replacement.
class ExportService {
  ExportService(this.dirs);
  final AppDirs dirs;

  /// Zips the given session directories into a single archive. Each session
  /// keeps its `<sessionId>/...` layout inside the zip, so the pipeline can
  /// unzip straight into its intake directory.
  Future<File> zipSessions(List<String> sessionIds) async {
    dirs.ensureCreated();
    final archive = Archive();
    for (final id in sessionIds) {
      final dir = Directory('${dirs.sessions.path}/$id');
      if (!dir.existsSync()) continue;
      for (final file in dir.listSync(recursive: true).whereType<File>()) {
        final rel = file.path.substring(dirs.sessions.path.length + 1);
        final bytes = await file.readAsBytes();
        archive.addFile(ArchiveFile(rel, bytes.length, bytes));
      }
    }
    final stamp = DateTime.now()
        .toUtc()
        .toIso8601String()
        .replaceAll(':', '')
        .split('.')
        .first;
    final out = File(
        '${dirs.exports.path}/nldea_${sessionIds.length}sessions_$stamp.zip');
    final bytes = ZipEncoder().encode(archive);
    await out.writeAsBytes(bytes!);
    return out;
  }

  /// Opens the platform share sheet for a zip produced by [zipSessions].
  Future<void> shareZip(File zip) async {
    await Share.shareXFiles(
      [XFile(zip.path, mimeType: 'application/zip')],
      subject: 'NLD-EA session export',
    );
  }
}

/// -----------------------------------------------------------------------
/// PHASE-2 STUB — direct upload to the encrypted on-prem store.
/// -----------------------------------------------------------------------
/// v1 hands sessions off via zip + share sheet only. When the campaign
/// backend endpoint exists, implement this interface (mTLS + resumable
/// upload + server-side checksum verification against checksums.json) and
/// swap it in behind the same provider. DO NOT wire any cloud storage here:
/// the DPIA commits to encrypted on-prem storage only.
abstract class UploadService {
  Future<UploadResult> uploadSession(String sessionId);
}

class UploadResult {
  const UploadResult({required this.sessionId, required this.accepted});
  final String sessionId;
  final bool accepted;
}

/// Placeholder implementation: always throws. Exists so call sites and the
/// provider seam are already in place for phase 2.
class StubUploadService implements UploadService {
  @override
  Future<UploadResult> uploadSession(String sessionId) {
    throw UnimplementedError(
        'Upload service is a later phase — use zip export (v1).');
  }
}
