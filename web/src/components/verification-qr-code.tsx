import { QRCode } from "antd";

export function VerificationQRCode({ value }: { value: string }) {
    return <QRCode value={value} size={208} bordered={false} color="#111111" bgColor="#ffffff" />;
}
