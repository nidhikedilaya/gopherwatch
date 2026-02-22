import { useState, useEffect } from 'react'

function App() {
  const [statusMap, setStatusMap] = useState({})
  const [alerts, setAlerts] = useState([])

  // Fetch data from your Go server
  const fetchDashboardData = async () => {
    try {
      // Fetch Live Status
      const statusRes = await fetch('/api/status')
      if (statusRes.ok) {
        const statusData = await statusRes.json()
        setStatusMap(statusData || {})
      }

      // Fetch Alert History
      const alertsRes = await fetch('/api/alerts/history')
      if (alertsRes.ok) {
        const alertsData = await alertsRes.json()
        setAlerts(alertsData || [])
      }
    } catch (err) {
      console.error("Error fetching data:", err)
    }
  }

  // Setup auto-refresh every 2 seconds
  useEffect(() => {
    fetchDashboardData()
    const interval = setInterval(fetchDashboardData, 1000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div style={{ padding: '20px', fontFamily: 'system-ui', backgroundColor: '#1e1e2f', color: '#fff', minHeight: '100vh' }}>
      <h1 style={{ borderBottom: '2px solid #3b82f6', paddingBottom: '10px' }}>👁️ GopherWatch Dashboard</h1>

      <div style={{ display: 'flex', gap: '20px', marginTop: '20px' }}>
        
        {/* LEFT COLUMN: LIVE STATUS */}
        <div style={{ flex: 1, backgroundColor: '#2a2a40', padding: '20px', borderRadius: '8px' }}>
          <h2>Live Agent Status</h2>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            {Object.keys(statusMap).length === 0 ? <p>No agents connected...</p> : null}
            
            {Object.entries(statusMap).map(([serviceId, report]) => {
              // Safely grab the snake_case keys from Go, defaulting to 0 if undefined
              const cpu = report.cpu_usage || 0;
              const memory = report.memory_usage || 0;
              const requests = report.request_count || 0;
              
              return (
                <div key={serviceId} style={{ backgroundColor: '#3b3b52', padding: '15px', borderRadius: '6px', borderLeft: '4px solid #10b981', marginBottom: '10px' }}>
                  <h3 style={{ margin: '0 0 10px 0', color: '#10b981' }}>{serviceId}</h3>
                  <p style={{ margin: 0 }}><strong>CPU:</strong> {cpu.toFixed(2)}%</p>
                  <p style={{ margin: 0 }}><strong>Memory:</strong> {memory.toFixed(2)} MB</p>
                  <p style={{ margin: 0 }}><strong>Requests:</strong> {requests}</p>
                  <p style={{ margin: 0, fontSize: '0.8em', color: '#9ca3af', marginTop: '5px' }}>Updated: {report.timestamp}</p>
                </div>
              );
            })}
          </div>
        </div>

        {/* RIGHT COLUMN: ALERT HISTORY */}
        <div style={{ flex: 1, backgroundColor: '#2a2a40', padding: '20px', borderRadius: '8px' }}>
          <h2>Critical Alerts (MySQL History)</h2>
          {alerts.length === 0 ? <p>No alerts triggered yet.</p> : (
            <table style={{ width: '100%', textAlign: 'left', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #4b5563' }}>
                  <th>ID</th>
                  <th>Service Name</th>
                  <th>Metric</th>
                  <th>Value</th>
                  <th>Time Triggered</th>
                </tr>
              </thead>
              <tbody>
                {alerts.map((alert) => (
                  <tr key={alert.id} style={{ borderBottom: '1px solid #3b3b52' }}>
                    <td style={{ padding: '10px 0' }}>#{alert.id}</td>
                    <td style={{ color: '#ef4444', fontWeight: 'bold' }}>{alert.service_name}</td>
                    <td>{alert.metric}</td>
                    <td>{alert.value.toFixed(2)}</td>
                    <td style={{ fontSize: '0.85em', color: '#9ca3af' }}>{new Date(alert.triggered_at).toLocaleTimeString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

      </div>
    </div>
  )
}

export default App