import type { Metadata } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'
import { QueryProvider } from '../lib/query-client'

const inter = Inter({ subsets: ['latin'] })

export const metadata: Metadata = {
  title: 'Nutreek - Weekly Nutrition Planner',
  description: 'Multi-tenant nutrition planning application',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en">
      <body className={inter.className}>
        <QueryProvider>
          {children}
        </QueryProvider>
      </body>
    </html>
  )
}