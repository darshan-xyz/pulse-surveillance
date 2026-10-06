import 'package:flutter/material.dart';
import 'viewer_screen.dart';

class DashboardScreen extends StatefulWidget {
  final String token;
  const DashboardScreen({Key? key, required this.token}) : super(key: key);

  @override
  _DashboardScreenState createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  // Mock data for MVP
  final List<Map<String, dynamic>> _devices = [
    {'id': 'mock-device-id', 'name': 'Living Room Server', 'status': 'online'}
  ];

  void _navigateToViewer(String deviceId) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ViewerScreen(deviceId: deviceId, token: widget.token),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        title: const Text('My Homes', style: TextStyle(color: Colors.white)),
        backgroundColor: Colors.grey[900],
        iconTheme: const IconThemeData(color: Colors.white),
      ),
      body: ListView.builder(
        padding: const EdgeInsets.all(16.0),
        itemCount: _devices.length,
        itemBuilder: (context, index) {
          final device = _devices[index];
          return Card(
            color: Colors.grey[900],
            shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
            child: ListTile(
              leading: const Icon(Icons.dns, color: Colors.blueAccent),
              title: Text(device['name'], style: const TextStyle(color: Colors.white)),
              subtitle: Text(
                device['status'], 
                style: TextStyle(color: device['status'] == 'online' ? Colors.greenAccent : Colors.redAccent),
              ),
              trailing: const Icon(Icons.chevron_right, color: Colors.grey),
              onTap: () => _navigateToViewer(device['id']),
            ),
          );
        },
      ),
    );
  }
}
