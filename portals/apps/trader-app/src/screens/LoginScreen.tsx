import { useAsgardeo } from '@asgardeo/react'
const logo = '/logo.png'
const hero = '/login_hero.png'
const gov_logo = '/gov_logo.png'

export function LoginScreen() {
  const { signIn } = useAsgardeo()

  return (
    <div className="min-h-screen flex flex-col lg:flex-row bg-white lg:overflow-hidden overflow-y-auto">
      {/* Left Section: Information & Branding */}
      <div className="w-full lg:w-[40%] flex flex-col justify-center px-8 lg:pl-36 lg:pr-6 py-12 lg:py-0 relative z-10 bg-white min-h-[500px] lg:min-h-screen">
        <div className="max-w-md mx-auto lg:mx-0 flex flex-col justify-center items-center lg:justify-start lg:items-start">
          <img src={logo} alt="OneTrade Logo" className="h-32 mb-5 xl:mb-10 object-contain" />

          <h1 className="text-2xl xl:text-3xl font-bold text-gray-900 leading-tight">Trade National Single Window</h1>

          <p className="mt-5 text-md text-gray-600 leading-relaxed text-center lg:text-left">
            A unified digital platform enabling seamless and secure trade facilitation services for traders, customs,
            and regulatory authorities.
          </p>
          <img src={gov_logo} alt="Government Logo" className="h-20 mt-5 object-contain opacity-80" />
        </div>
      </div>

      {/* Right Section: Hero & Authentication */}
      <div className="relative flex-1 min-h-[500px] lg:min-h-screen overflow-hidden">
        {/* Slanted Hero Background with Overlay */}
        <div className="absolute inset-0 [clip-path:none] lg:[clip-path:polygon(25%_0,100%_0,100%_100%,0%_100%)]">
          <div className="absolute inset-0 bg-cover bg-center" style={{ backgroundImage: `url(${hero})` }} />
          <div className="absolute inset-0 bg-black/50" />
        </div>

        {/* Centered Interaction Area: Dark Card Style */}
        <div className="absolute inset-0 flex flex-col items-center justify-center px-6">
          <div className="bg-secondary-950/80 border border-white/10 py-10 px-8 xl:px-12 rounded-2xl flex flex-col xl:flex-row items-center gap-6 xl:gap-10 shadow-[0_20px_50px_rgba(0,0,0,0.5)]">
            {/* Content Section */}
            <div className="flex flex-col xl:flex-row items-center gap-8 xl:gap-12">
              <div className="flex flex-col items-center xl:items-start text-center xl:text-left">
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
