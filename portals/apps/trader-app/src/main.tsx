import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import "@radix-ui/themes/styles.css";
import {BrowserRouter} from 'react-router-dom';
import {Theme} from '@radix-ui/themes';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Theme scaling="110%">
      <BrowserRouter>
        <App/>
      </BrowserRouter>
    </Theme>
  </StrictMode>,
)
