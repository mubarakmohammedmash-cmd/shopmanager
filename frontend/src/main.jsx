import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import Gate from './Gate.jsx'

const theme =
  localStorage.getItem('theme') ||
  (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
document.documentElement.setAttribute('data-theme', theme)

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <Gate />
  </StrictMode>,
)
