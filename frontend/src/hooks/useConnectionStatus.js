import { useEffect, useState } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { GetConnectionStatus } from '../../wailsjs/go/main/App'

export default function useConnectionStatus() {
  const [status, setStatus] = useState('disconnected')

  useEffect(() => {
    GetConnectionStatus().then(s => setStatus(s || 'disconnected'))
    const off = EventsOn('eventsub:status', s => setStatus(s || 'disconnected'))
    return () => off()
  }, [])

  return status
}
