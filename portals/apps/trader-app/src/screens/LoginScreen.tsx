import { useAsgardeo } from '@asgardeo/react'
const logo = '/logo.png'
const hero = '/login_hero.png'

export function LoginScreen() {
  const { signIn } = useAsgardeo()

  return (
    <div className="min-h-screen flex flex-row bg-white overflow-hidden">
      {/* Left Section: Information & Branding */}
      <div className="w-[40%] flex flex-col justify-center pl-36 pr-3 py-12 relative z-10 bg-white">
        <div className="max-w-md">
          <img src={logo} alt="OneTrade_Logo" className="h-32 mb-12 object-contain" />

          <h1 className="text-3xl font-bold text-gray-900 leading-tight">Trade National Single Window</h1>

          <p className="mt-6 text-md text-gray-600 leading-relaxed">
            A unified digital platform enabling seamless and secure trade facilitation services for traders, customs,
            and regulatory authorities.
          </p>
        </div>
      </div>

      {/* Right Section: Hero & Authentication */}
      <div className="relative flex-1 min-h-screen overflow-hidden">
        {/* Slanted Hero Background */}
        <div
          className="absolute inset-0 bg-cover bg-center"
          style={{
            backgroundImage: `url(${hero})`,
            clipPath: 'polygon(25% 0, 100% 0, 100% 100%, 0% 100%)',
          }}
        />

        {/* Dark Overlay for readability */}
        <div
          className="absolute inset-0 bg-black/50"
          style={{
            clipPath: 'polygon(25% 0, 100% 0, 100% 100%, 0% 100%)',
          }}
        />

        {/* Centered Interaction Area: Dark Card Style */}
        <div className="absolute inset-0 flex flex-col items-center justify-center px-6">
          <div className="bg-[#020617]/80 border border-white/10 py-10 px-12 rounded-4xl flex flex-row items-center gap-10 shadow-[0_20px_50px_rgba(0,0,0,0.5)]">
            {/* Content Section */}
            <div className="flex flex-row items-center gap-12">
              <div className="flex flex-col">
                <h2 className="text-2xl font-bold text-white tracking-wide">Trader Portal</h2>
                <p className="text-white/60 text-xs mt-1">Sign in to continue to your consignments.</p>
              </div>

              <button
                onClick={() => {
                  void signIn()
                }}
                className="bg-[#6366f1] hover:bg-[#4f46e5] text-white px-10 py-2.5 rounded-2xl text-lg font-bold transition-all hover:scale-105 active:scale-95 shadow-lg cursor-pointer"
              >
                Sign In
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
