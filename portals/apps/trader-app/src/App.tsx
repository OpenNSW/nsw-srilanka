import {Routes, Route, Navigate} from 'react-router-dom'
import './App.css'
import {Layout} from './components/Layout'
import {ConsignmentScreen} from "./screens/ConsignmentScreen.tsx"
import {ConsignmentDetailScreen} from "./screens/ConsignmentDetailScreen.tsx"
import {TaskDetailScreen} from "./screens/TaskDetailScreen.tsx";
import {PreconsignmentScreen} from "./screens/PreconsignmentScreen.tsx"

function App() {
  return (
    <Routes>
      <Route element={<Layout/>}>
        <Route path="/" element={<Navigate to="/consignments" replace/>}/>
        <Route path="/consignments" element={<ConsignmentScreen/>}/>
        <Route path="/consignments/:consignmentId" element={<ConsignmentDetailScreen/>}/>
        <Route path="/consignments/:consignmentId/tasks/:taskId" element={<TaskDetailScreen/>}/>
        <Route path="/pre-consignments" element={<PreconsignmentScreen/>}/>
        <Route path="/pre-consignments/:preConsignmentId/tasks/:taskId" element={<TaskDetailScreen/>}/>
      </Route>
    </Routes>
  )
}

export default App