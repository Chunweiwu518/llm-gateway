import UsageTable from '../components/UsageTable'

export default function Usage() {
  return (
    <>
      <h1>Usage</h1>
      <UsageTable endpoint="/api/usage" />
    </>
  )
}
