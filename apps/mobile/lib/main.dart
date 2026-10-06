import 'package:flutter/material.dart';
import 'screens/login_screen.dart';

void main() {
  runApp(const HSSMobileApp());
}

class HSSMobileApp extends StatelessWidget {
  const HSSMobileApp({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'HSS Mobile',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        brightness: Brightness.dark,
        primarySwatch: Colors.blue,
        visualDensity: VisualDensity.adaptivePlatformDensity,
      ),
      home: LoginScreen(),
    );
  }
}
