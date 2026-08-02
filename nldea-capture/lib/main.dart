import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path_provider/path_provider.dart';

import 'app.dart';
import 'core/providers.dart';
import 'core/services/stores.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // All campaign output lives under the app documents directory:
  // sessions/ (media + manifests), consents/ (consent records, kept
  // separate from media), exports/ (zips for hand-off).
  final docs = await getApplicationDocumentsDirectory();
  final dirs = AppDirs(Directory('${docs.path}/nldea'))..ensureCreated();
  runApp(
    ProviderScope(
      overrides: [appDirsProvider.overrideWithValue(dirs)],
      child: const NldeaApp(),
    ),
  );
}
