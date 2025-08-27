import Link from 'next/link'

export default function Home() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-between p-24">
      <div className="z-10 w-full max-w-5xl items-center justify-between font-mono text-sm lg:flex">
        <h1 className="text-4xl font-bold">Nutreek</h1>
        <p className="text-lg">Weekly Nutrition Planning App</p>
      </div>

      <div className="relative flex place-items-center">
        <div className="text-center">
          <h2 className="text-2xl font-semibold mb-4">Welcome to Nutreek</h2>
          <p className="text-gray-600 mb-8">
            Plan your weekly meals, manage household nutrition, and create shopping lists with ease.
          </p>
          <div className="space-x-4">
            <Link
              href="/auth/login"
              className="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded"
            >
              Login
            </Link>
            <Link
              href="/auth/register"
              className="bg-green-500 hover:bg-green-700 text-white font-bold py-2 px-4 rounded"
            >
              Register
            </Link>
          </div>
        </div>
      </div>

      <div className="mb-32 grid text-center lg:mb-0 lg:grid-cols-3 lg:text-left">
        <div className="group rounded-lg border border-transparent px-5 py-4 transition-colors hover:border-gray-300 hover:bg-gray-100">
          <h3 className="mb-3 text-lg font-semibold">
            Multi-tenant households
          </h3>
          <p className="m-0 max-w-[30ch] text-sm opacity-50">
            Manage nutrition planning for your entire household with shared meal plans.
          </p>
        </div>

        <div className="group rounded-lg border border-transparent px-5 py-4 transition-colors hover:border-gray-300 hover:bg-gray-100">
          <h3 className="mb-3 text-lg font-semibold">
            Weekly meal planning
          </h3>
          <p className="m-0 max-w-[30ch] text-sm opacity-50">
            Plan your meals for the entire week with our intuitive planner interface.
          </p>
        </div>

        <div className="group rounded-lg border border-transparent px-5 py-4 transition-colors hover:border-gray-300 hover:bg-gray-100">
          <h3 className="mb-3 text-lg font-semibold">
            Smart shopping lists
          </h3>
          <p className="m-0 max-w-[30ch] text-sm opacity-50">
            Automatically generate shopping lists based on your weekly meal plan.
          </p>
        </div>
      </div>
    </main>
  )
}