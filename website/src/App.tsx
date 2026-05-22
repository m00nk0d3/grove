import { Nav } from '@/components/Nav'
import { Hero } from '@/components/Hero'
import { Problems } from '@/components/Problems'
import { Features } from '@/components/Features'
import { Themes } from '@/components/Themes'
import { Keybindings } from '@/components/Keybindings'
import { Install } from '@/components/Install'
import { Changelog } from '@/components/Changelog'
import { Footer } from '@/components/Footer'

function App() {
  return (
    <div className="min-h-screen bg-[#0a0e27] text-[#e2e8f0] antialiased">
      <Nav />
      <main>
        <Hero />
        <Problems />
        <Features />
        <Themes />
        <Keybindings />
        <Install />
        <Changelog />
      </main>
      <Footer />
    </div>
  )
}

export default App
