import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/providers.dart';
import '../features/capture/screens/guided_capture_screen.dart';
import '../features/capture/screens/monk_scale_screen.dart';
import '../features/capture/screens/review_screen.dart';
import '../features/capture/screens/session_setup_screen.dart';
import '../features/consent/screens/consent_screen.dart';
import '../features/consent/screens/withdrawal_screen.dart';
import '../features/dashboard/screens/dashboard_screen.dart';
import '../features/home/screens/home_screen.dart';
import '../features/sessions/screens/sessions_screen.dart';

/// Routes that are hard-gated on recorded consent. Navigation to any of
/// these without an active [activeConsentProvider] record redirects to
/// /consent — the DPIA's "consent before any capture" constraint, enforced
/// at the router so no screen can be deep-linked around it.
const Set<String> consentGatedPaths = {
  '/setup',
  '/skin-tone',
  '/capture',
  '/review',
};

final goRouterProvider = Provider<GoRouter>((ref) {
  return GoRouter(
    initialLocation: '/',
    redirect: (context, state) {
      final path = state.matchedLocation;
      if (consentGatedPaths.contains(path) &&
          ref.read(activeConsentProvider) == null) {
        return '/consent';
      }
      return null;
    },
    routes: [
      GoRoute(path: '/', builder: (_, __) => const HomeScreen()),
      GoRoute(path: '/consent', builder: (_, __) => const ConsentScreen()),
      GoRoute(path: '/withdraw', builder: (_, __) => const WithdrawalScreen()),
      GoRoute(path: '/setup', builder: (_, __) => const SessionSetupScreen()),
      GoRoute(path: '/skin-tone', builder: (_, __) => const MonkScaleScreen()),
      GoRoute(
          path: '/capture', builder: (_, __) => const GuidedCaptureScreen()),
      GoRoute(path: '/review', builder: (_, __) => const ReviewScreen()),
      GoRoute(path: '/dashboard', builder: (_, __) => const DashboardScreen()),
      GoRoute(path: '/sessions', builder: (_, __) => const SessionsScreen()),
    ],
  );
});
