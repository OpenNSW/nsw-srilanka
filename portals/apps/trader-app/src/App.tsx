import { Routes, Route } from 'react-router-dom'
import './App.css'
import { Layout } from './components/Layout'
import { DashboardScreen } from "./screens/DashboardScreen.tsx"
import { ConsignmentDetailScreen } from "./screens/ConsignmentDetailScreen.tsx"
import {TaskDetailScreen} from "./screens/TaskDetailScreen.tsx";

function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<DashboardScreen />} />
        <Route path="/consignments" element={<DashboardScreen />} />
        <Route path="/consignments/:consignmentId" element={<ConsignmentDetailScreen />} />
        <Route path="/consignments/:consignmentId/tasks/:taskId" element={<TaskDetailScreen />} />
      </Route>
    </Routes>
  )
}

export default App