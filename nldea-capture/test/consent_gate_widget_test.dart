import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nldea_capture/app.dart';
import 'package:nldea_capture/core/providers.dart';
import 'package:nldea_capture/core/services/capture_camera.dart';
import 'package:nldea_capture/core/services/stores.dart';
import 'package:nldea_capture/routing/app_router.dart';

/// The consent gate is the DPIA's hard constraint: no capture route is
/// reachable until the subject has agreed on the consent screen.
void main() {
  late Directory tmp;

  setUp(() {
    tmp = Directory.systemTemp.createTempSync('nldea_gate_');
  });

  tearDown(() {
    tmp.deleteSync(recursive: true);
  });

  Widget app(ProviderContainer container) => UncontrolledProviderScope(
        container: container,
        child: const NldeaApp(),
      );

  ProviderContainer makeContainer() => ProviderContainer(overrides: [
        appDirsProvider.overrideWithValue(AppDirs(tmp)..ensureCreated()),
        captureCameraProvider.overrideWithValue(FakeCaptureCamera()),
      ]);

  testWidgets('new session leads to the consent screen first',
      (tester) async {
    final container = makeContainer();
    addTearDown(container.dispose);
    await tester.pumpWidget(app(container));
    await tester.pumpAndSettle();

    await tester.tap(find.text('New subject session'));
    await tester.pumpAndSettle();

    expect(find.text('Participant consent'), findsOneWidget);
    expect(find.text('I agree'), findsOneWidget);
    // Capture setup is NOT shown.
    expect(find.text('Session setup'), findsNothing);
  });

  testWidgets('deep-linking any capture route without consent redirects '
      'back to consent', (tester) async {
    final container = makeContainer();
    addTearDown(container.dispose);
    await tester.pumpWidget(app(container));
    await tester.pumpAndSettle();

    final router = container.read(goRouterProvider);
    for (final path in consentGatedPaths) {
      router.go(path);
      await tester.pumpAndSettle();
      expect(find.text('Participant consent'), findsOneWidget,
          reason: '$path must be consent-gated');
      expect(
          router.routerDelegate.currentConfiguration.uri.path, '/consent');
    }
  });

  testWidgets('"I agree" requires the confirmation checkbox', (tester) async {
    final container = makeContainer();
    addTearDown(container.dispose);
    await tester.pumpWidget(app(container));
    await tester.pumpAndSettle();
    container.read(goRouterProvider).go('/consent');
    await tester.pumpAndSettle();

    final agreeButton = find.widgetWithText(FilledButton, 'I agree');
    expect(tester.widget<FilledButton>(agreeButton).onPressed, isNull);
  });

  testWidgets('after agreeing, the capture flow opens with a pseudonymous '
      'subject and a stored consent record', (tester) async {
    final container = makeContainer();
    addTearDown(container.dispose);
    await tester.pumpWidget(app(container));
    await tester.pumpAndSettle();
    container.read(goRouterProvider).go('/consent');
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextField), 'OP1');
    await tester.tap(find.byType(CheckboxListTile));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'I agree'));
    await tester.pumpAndSettle();

    // Now on session setup, showing the generated pseudonym.
    expect(find.text('Session setup'), findsOneWidget);
    final consent = container.read(activeConsentProvider);
    expect(consent, isNotNull);
    expect(consent!.subjectId, matches(RegExp(r'^NLD-[A-Z2-9]{8}$')));
    expect(find.textContaining(consent.subjectId), findsOneWidget);

    // Consent record persisted separately from media.
    final stored =
        await container.read(consentStoreProvider).forSubject(consent.subjectId);
    expect(stored.single.consentId, consent.consentId);
    expect(stored.single.operatorId, 'OP1');
    expect(stored.single.isWithdrawn, isFalse);
  });

  testWidgets('declining returns home and keeps the gate shut',
      (tester) async {
    final container = makeContainer();
    addTearDown(container.dispose);
    await tester.pumpWidget(app(container));
    await tester.pumpAndSettle();
    container.read(goRouterProvider).go('/consent');
    await tester.pumpAndSettle();

    await tester.tap(find.text('Decline'));
    await tester.pumpAndSettle();

    expect(find.text('NLD-EA Capture'), findsOneWidget);
    expect(container.read(activeConsentProvider), isNull);

    container.read(goRouterProvider).go('/capture');
    await tester.pumpAndSettle();
    expect(find.text('Participant consent'), findsOneWidget);
  });
}
