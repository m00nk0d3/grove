import { motion } from 'framer-motion'
import { LiveDemo } from './LiveDemo'

export function Demo() {
  return (
    <section id="demo" className="px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="mb-14 text-center"
        >
          <p className="mb-3 font-mono text-sm text-[#00d9ff]">// see it in action</p>
          <h2 className="text-4xl font-bold tracking-tight">Live demo</h2>
          <p className="mt-4 text-[#4a5568]">
            Every screen here was rendered by Grove itself — from mission control to a
            workflow's report to a theme switch. Watch it, or drive it with the keys.
          </p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 32, scale: 0.98 }}
          whileInView={{ opacity: 1, y: 0, scale: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 0.7, ease: [0.16, 1, 0.3, 1] }}
        >
          <LiveDemo />
        </motion.div>
      </div>
    </section>
  )
}
