import { BrowserRouter, Route, Routes } from 'react-router-dom'
import Home from './pages/Home'
import Activation from './pages/Activation'
import './App.css'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/activation/:code" element={<Activation />} />
        <Route path="*" element={<Home />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
