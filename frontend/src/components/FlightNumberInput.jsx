import BoxedCharInput from './BoxedCharInput';

// Flight number input — thin wrapper around BoxedCharInput that enforces
// airline-code conventions: unlimited length, uppercase, alphanumeric + space.
export default function FlightNumberInput({ value, onChange, ariaLabel }) {
  return (
    <BoxedCharInput
      value={value}
      onChange={onChange}
      uppercase
      sanitize={(s) => s.replace(/[^A-Z0-9 ]/g, '')}
      ariaLabel={ariaLabel || 'Номер рейса'}
    />
  );
}
