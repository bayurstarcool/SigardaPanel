<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { auth } from '$lib/auth.svelte';

  let { 
    onNotification = (notification: any) => {},
    onConnect = () => {},
    onDisconnect = () => {},
    reconnectInterval = 5000,
    maxReconnectAttempts = 10
  } = $props();

  let ws = $state<WebSocket | null>(null);
  let connected = $state(false);
  let reconnectAttempts = $state(0);
  let reconnectTimer = $state<ReturnType<typeof setTimeout>>();

  onMount(() => {
    if (!auth.token) return;
    connect();
  });

  onDestroy(() => {
    disconnect();
  });

  function connect() {
    if (ws) {
      ws.close();
    }

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${window.location.host}/api/ws/notifications?token=${auth.token}`;

    ws = new WebSocket(url);

    ws.onopen = () => {
      connected = true;
      reconnectAttempts = 0;
      onConnect();
    };

    ws.onmessage = (event) => {
      try {
        const notification = JSON.parse(event.data);
        onNotification(notification);
      } catch (e) {
        console.error('Failed to parse notification:', e);
      }
    };

    ws.onclose = () => {
      connected = false;
      onDisconnect();
      scheduleReconnect();
    };

    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      connected = false;
    };
  }

  function disconnect() {
    if (ws) {
      ws.close();
      ws = null;
    }
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
    }
  }

  function scheduleReconnect() {
    if (reconnectAttempts >= maxReconnectAttempts) {
      console.warn('Max reconnect attempts reached');
      return;
    }

    reconnectTimer = setTimeout(() => {
      reconnectAttempts++;
      connect();
    }, reconnectInterval * Math.pow(1.5, reconnectAttempts)); // Exponential backoff
  }

  function send(data: any) {
    if (ws && connected) {
      ws.send(JSON.stringify(data));
    }
  }
</script>

<div class="ws-status" class:connected>
  <span class="status-dot"></span>
  <span class="status-text">{connected ? 'Connected' : 'Disconnected'}</span>
</div>

<style>
  .ws-status {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 8px;
    border-radius: 6px;
    font-size: 0.75rem;
  }
  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #ef4444;
    transition: background 0.2s;
  }
  .ws-status.connected .status-dot {
    background: #22c55e;
  }
  .status-text {
    color: #6b7280;
  }
  .ws-status.connected .status-text {
    color: #22c55e;
  }
</style>
