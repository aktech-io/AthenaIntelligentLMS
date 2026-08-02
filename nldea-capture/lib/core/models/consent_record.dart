/// Consent records are stored SEPARATELY from media (data minimization per
/// the DPIA design, docs/nemo/10 §Part 2 G1): the session directories carry
/// only the pseudonymous subjectId + consentId; this record is the link that
/// proves consent and enables withdrawal-by-subject-ID.
///
/// Deliberately contains NO name, phone number, or other direct identifiers.
/// The paper/tablet-signed consent form (kept by the field lead) maps the
/// subject to their pseudonym; this app never stores that mapping.
class ConsentRecord {
  const ConsentRecord({
    required this.consentId,
    required this.subjectId,
    required this.consentTextVersion,
    required this.operatorId,
    required this.agreedAt,
    this.withdrawnAt,
  });

  final String consentId;
  final String subjectId;

  /// Version tag of the consent copy shown, e.g. `v0-draft`. Bump whenever
  /// the consent text changes so records stay auditable.
  final String consentTextVersion;

  /// Field operator running the session (initials/staff code, not the subject).
  final String operatorId;
  final DateTime agreedAt;
  final DateTime? withdrawnAt;

  bool get isWithdrawn => withdrawnAt != null;

  ConsentRecord withdrawn(DateTime at) => ConsentRecord(
        consentId: consentId,
        subjectId: subjectId,
        consentTextVersion: consentTextVersion,
        operatorId: operatorId,
        agreedAt: agreedAt,
        withdrawnAt: at,
      );

  Map<String, Object?> toJson() => {
        'consentId': consentId,
        'subjectId': subjectId,
        'consentTextVersion': consentTextVersion,
        'operatorId': operatorId,
        'agreedAt': agreedAt.toUtc().toIso8601String(),
        'withdrawnAt': withdrawnAt?.toUtc().toIso8601String(),
      };

  factory ConsentRecord.fromJson(Map<String, Object?> json) => ConsentRecord(
        consentId: json['consentId'] as String,
        subjectId: json['subjectId'] as String,
        consentTextVersion: json['consentTextVersion'] as String,
        operatorId: json['operatorId'] as String,
        agreedAt: DateTime.parse(json['agreedAt'] as String),
        withdrawnAt: json['withdrawnAt'] == null
            ? null
            : DateTime.parse(json['withdrawnAt'] as String),
      );
}

/// Current consent copy. PLACEHOLDER — MUST be reviewed and replaced by DPIA
/// counsel before gate G1 (no field capture before the DPIA is filed with
/// the ODPC). Bump [consentTextVersion] on any change.
const String consentTextVersion = 'v0-draft-pending-dpia-review';

const String consentText = '''
[PLACEHOLDER CONSENT COPY — PENDING DPIA COUNSEL REVIEW. DO NOT USE IN THE
FIELD UNTIL REPLACED WITH THE COUNSEL-APPROVED TEXT AND THE DPIA IS FILED.]

Nemo Liveness Dataset (East Africa) — Participant Consent

1. Purpose. Short video clips of your face will be recorded to develop and
   test anti-fraud (liveness detection) technology. Your clips will be used
   ONLY for developing anti-fraud models — never for advertising, and never
   shared or sold to third parties.

2. What is stored. Your clips are stored under a random code (for example
   "NLD-7Q2KX9BM"), not your name. Keep your participant card: the code on
   it is the only way your data is identified.

3. Compensation. You will receive KES 800 for a completed session
   (about 15 minutes).

4. Withdrawal. You may withdraw at any time, without giving a reason, by
   quoting your participant code. All of your clips will then be deleted.

5. Retention. Clips are kept for a maximum of 36 months on encrypted
   servers in-country, then deleted.

6. Eligibility. You must be 18 or older. Minors are excluded.

By tapping "I agree", you confirm you have understood the above and agree
to take part.
''';
