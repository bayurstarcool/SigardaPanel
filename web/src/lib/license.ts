// Client-side license state
let _tier = 'free';
let _features: Record<string, boolean> = {};
let _listeners: (() => void)[] = [];

export const licenseStore = {
  get tier() { return _tier; },
  get features() { return _features; },

  async fetch() {
    try {
      const token = localStorage.getItem('sigarda_token');
      const res = await fetch('/api/proxy/license', {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        _tier = data.tier || 'free';
        _features = data.features || {};
        _listeners.forEach(fn => fn());
      }
    } catch {
      // Default to free tier
    }
  },

  hasFeature(key: string): boolean {
    return _features[key] === true;
  },

  isPro(): boolean {
    return _tier === 'pro' || _tier === 'enterprise';
  },

  isEnterprise(): boolean {
    return _tier === 'enterprise';
  },

  subscribe(fn: () => void) {
    _listeners.push(fn);
    return () => { _listeners = _listeners.filter(l => l !== fn); };
  }
};
