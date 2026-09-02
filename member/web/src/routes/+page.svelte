<script>
  import { Shield, ArrowRight } from 'lucide-svelte';
</script>

<div class="min-h-screen bg-[#060d1f] text-white flex items-center justify-center p-4">
  <div class="max-w-4xl mx-auto text-center">
    <div class="inline-flex items-center gap-2 px-4 py-1.5 rounded-full border border-sky-400/20 bg-sky-400/5 text-sky-300 text-xs font-medium mb-6">
      <span class="w-1.5 h-1.5 rounded-full bg-sky-400 animate-pulse"></span>
      Member Portal
    </div>
    <h1 class="text-5xl sm:text-6xl font-extrabold tracking-tight mb-6">
      <span class="bg-gradient-to-r from-white via-sky-200 to-indigo-300 bg-clip-text text-transparent">SigardaPanel</span>
    </h1>
    <p class="text-lg text-slate-400 max-w-xl mx-auto mb-10">
      Manage your account, license, and API access.
    </p>
    <div class="flex flex-wrap justify-center gap-4">
      <a href="/login" class="px-8 py-3.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)] transition-all">
        Sign In
      </a>
      <a href="/register" class="px-8 py-3.5 rounded-xl text-sm font-medium border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] text-white transition-all">
        Create Account
      </a>
    </div>
  </div>
</div>
