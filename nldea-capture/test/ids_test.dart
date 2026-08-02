import 'dart:math';

import 'package:flutter_test/flutter_test.dart';
import 'package:nldea_capture/core/services/ids.dart';

void main() {
  group('ID generation', () {
    test('session ids are valid v4 UUIDs', () {
      for (var i = 0; i < 200; i++) {
        expect(generateSessionId(), matches(uuidV4Pattern));
      }
    });

    test('consent ids are C-<uuid v4>', () {
      final id = generateConsentId();
      expect(id, startsWith('C-'));
      expect(id.substring(2), matches(uuidV4Pattern));
    });

    test('subject pseudonyms match NLD-XXXXXXXX with safe alphabet', () {
      for (var i = 0; i < 200; i++) {
        final id = generateSubjectId();
        expect(id, matches(subjectIdPattern));
        // Ambiguous characters excluded so card-copied IDs survive.
        expect(id.substring(4), isNot(matches(RegExp('[01OIL]'))));
      }
    });

    test('generation is collision-free over many draws', () {
      final subjects = {for (var i = 0; i < 5000; i++) generateSubjectId()};
      final uuids = {for (var i = 0; i < 5000; i++) generateUuidV4()};
      expect(subjects.length, 5000);
      expect(uuids.length, 5000);
    });

    test('injectable Random gives deterministic output for audits', () {
      expect(generateSubjectId(Random(7)), generateSubjectId(Random(7)));
      expect(generateUuidV4(Random(7)), generateUuidV4(Random(7)));
      // ...and the deterministic UUID is still a valid v4.
      expect(generateUuidV4(Random(7)), matches(uuidV4Pattern));
    });

    test('pseudonyms carry no session/time information', () {
      // Two ids generated back-to-back share no structure beyond the prefix.
      final a = generateSubjectId();
      final b = generateSubjectId();
      expect(a.substring(0, 4), 'NLD-');
      expect(b.substring(0, 4), 'NLD-');
      expect(a, isNot(b));
    });
  });
}
