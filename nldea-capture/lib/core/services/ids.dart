import 'dart:math';

/// ID generation for the NLD-EA campaign.
///
/// - Session IDs are RFC-4122 v4 UUIDs (the manifest contract).
/// - Subject IDs are short pseudonyms (`NLD-XXXXXXXX`) a participant can
///   read off a card to withdraw. No PII goes into them: they are purely
///   random, drawn from an unambiguous alphabet (no 0/O, 1/I/L).
/// - Consent IDs are UUID-based with a `C-` prefix.
///
/// All generators accept an injectable [Random] for deterministic tests;
/// production callers use the default cryptographically secure source.

final Random _secure = Random.secure();

/// RFC-4122 version 4 UUID, lowercase.
String generateUuidV4([Random? random]) {
  final rng = random ?? _secure;
  final bytes = List<int>.generate(16, (_) => rng.nextInt(256));
  bytes[6] = (bytes[6] & 0x0f) | 0x40; // version 4
  bytes[8] = (bytes[8] & 0x3f) | 0x80; // variant 10xx
  final hex =
      bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
  return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-'
      '${hex.substring(12, 16)}-${hex.substring(16, 20)}-'
      '${hex.substring(20)}';
}

/// Alphabet for subject pseudonyms: unambiguous when read aloud or copied
/// from a participant card (no 0/O, 1/I/L).
const String subjectIdAlphabet = 'ABCDEFGHJKMNPQRSTUVWXYZ23456789';

/// Pseudonymous subject ID, e.g. `NLD-7Q2KX9BM`. 31^8 ≈ 8.5e11 combinations,
/// so collisions across a 400-subject campaign are negligible (and the
/// consent store still rejects duplicates defensively).
String generateSubjectId([Random? random]) {
  final rng = random ?? _secure;
  final code = List.generate(
      8, (_) => subjectIdAlphabet[rng.nextInt(subjectIdAlphabet.length)]).join();
  return 'NLD-$code';
}

/// Consent record ID, e.g. `C-<uuid>`.
String generateConsentId([Random? random]) => 'C-${generateUuidV4(random)}';

/// Session ID: bare v4 UUID per the manifest contract.
String generateSessionId([Random? random]) => generateUuidV4(random);

final RegExp uuidV4Pattern = RegExp(
    r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$');

final RegExp subjectIdPattern =
    RegExp('^NLD-[$subjectIdAlphabet]{8}\$');
