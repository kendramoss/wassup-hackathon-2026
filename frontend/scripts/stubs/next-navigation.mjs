export const useParams = () => ({});
export const useRouter = () => ({ push() {}, replace() {}, back() {}, refresh() {}, prefetch() {} });
export const usePathname = () => "/";
export const useSearchParams = () => new URLSearchParams();
export const notFound = () => { throw new Error("notFound"); };
export const redirect = () => { throw new Error("redirect"); };
